// Package importer runs one supplier delivery through the pipeline:
// store the original file, version the raw records, update the supplier
// items, detect changes, let the rules decide and create actions and tasks.
package importer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/actions"
	"velopim/internal/availability"
	"velopim/internal/catalog"
	"velopim/internal/connector"
	"velopim/internal/db"
	"velopim/internal/pricing"
	"velopim/internal/rules"
)

// Options configure one run.
type Options struct {
	// RawDir is where the unchanged original file is kept.
	RawDir string
	// MaxDropPct holds the import when more than this share of the active
	// articles is missing. Zero means 20.
	MaxDropPct float64
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// Result summarizes one run.
type Result struct {
	RunID           int64
	Status          string // "done" or "held"
	Records         int
	New             int
	Changed         int
	Removed         int
	Issues          int
	QuantityChanges int
	AutoApplied     int
	PendingApproval int
	// Ignored counts changes the usage policy says to ignore.
	Ignored int
}

const maxStoredIssues = 500

// Run imports one delivery of the connector's source. Everything happens in
// one transaction: a failed run leaves no partial state behind.
func Run(ctx context.Context, pool *pgxpool.Pool, conn connector.Connector, r io.Reader, opt Options) (Result, error) {
	now := time.Now
	if opt.Now != nil {
		now = opt.Now
	}
	if opt.MaxDropPct == 0 {
		opt.MaxDropPct = 20
	}
	sourceID := conn.SourceID()

	data, err := io.ReadAll(r)
	if err != nil {
		return Result{}, fmt.Errorf("importer: reading delivery: %w", err)
	}
	sum := sha256.Sum256(data)
	fileHash := hex.EncodeToString(sum[:])
	filePath, err := storeOriginal(opt.RawDir, sourceID, fileHash, now(), data)
	if err != nil {
		return Result{}, err
	}

	parsed, err := conn.Parse(bytes.NewReader(data))
	if err != nil {
		return Result{}, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	q := db.New(tx)
	mappingJSON, err := q.GetAvailabilityMapping(ctx, sourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Result{}, fmt.Errorf("importer: source %q is not configured", sourceID)
		}
		return Result{}, err
	}
	var mapping []availability.MappingRule
	if err := json.Unmarshal(mappingJSON, &mapping); err != nil {
		return Result{}, fmt.Errorf("importer: availability mapping of %q: %w", sourceID, err)
	}
	ruleSet, err := loadRules(ctx, q)
	if err != nil {
		return Result{}, err
	}
	priceRules, err := loadPricingRules(ctx, q)
	if err != nil {
		return Result{}, err
	}
	policies, err := catalog.LoadUsagePolicies(ctx, q)
	if err != nil {
		return Result{}, err
	}
	prev, err := loadItems(ctx, q, sourceID)
	if err != nil {
		return Result{}, err
	}

	res := Result{Status: "done", Records: len(parsed.Records)}
	res.RunID, err = q.CreateImportRun(ctx, db.CreateImportRunParams{
		SourceID: sourceID, FilePath: &filePath, FileSha256: &fileHash,
		RecordCount: int32(len(parsed.Records)), StartedAt: now()})
	if err != nil {
		return Result{}, err
	}
	run := &runState{ctx: ctx, q: q, runID: res.RunID, sourceID: sourceID, rules: ruleSet, priceRules: priceRules, policies: policies, res: &res}

	issues := append([]connector.Issue(nil), parsed.Issues...)
	seen := make(map[string]bool, len(parsed.Records))
	for _, rec := range parsed.Records {
		seen[rec.Key] = true
		item, itemIssues := conn.Normalize(rec)
		issues = append(issues, itemIssues...)
		item.Availability = availability.Map(rec.Fields, mapping, sourceID, now())
		if err := run.upsert(rec, item, prev[rec.Key]); err != nil {
			return Result{}, fmt.Errorf("importer: article %s: %w", rec.Key, err)
		}
	}

	if err := run.decideNewModels(); err != nil {
		return Result{}, err
	}

	// Articles missing from the delivery.
	var active, missing []string
	for sku, p := range prev {
		if p.active {
			active = append(active, sku)
			if !seen[sku] {
				missing = append(missing, sku)
			}
		}
	}
	if connector.SuspiciousDrop(len(active), len(active)-len(missing), opt.MaxDropPct) {
		res.Status = "held"
		if err := run.task("import_held", 1,
			fmt.Sprintf("Import prüfen: %d von %d Artikeln fehlen", len(missing), len(active)),
			"Die Lieferung ist vermutlich unvollständig. Es wurde nichts deaktiviert.", nil, nil); err != nil {
			return Result{}, err
		}
	} else {
		for _, p := range actions.ProposeRemoved(active, keys(seen)) {
			it := prev[p.SKU]
			if err := q.DeactivateSupplierItem(ctx, it.id); err != nil {
				return Result{}, err
			}
			if err := run.fieldChange(it.id, "active", "true", "false"); err != nil {
				return Result{}, err
			}
			if err := run.decide(p, it.id, it.item, nil); err != nil {
				return Result{}, err
			}
			res.Removed++
		}
	}

	res.Issues = len(issues)
	if len(issues) > 0 {
		if err := run.task("data_issue", 3, fmt.Sprintf("%d Datenprobleme im Import", len(issues)),
			"Details stehen in der Zusammenfassung des Importlaufs.", nil, nil); err != nil {
			return Result{}, err
		}
	}
	if len(issues) > maxStoredIssues {
		issues = issues[:maxStoredIssues]
	}
	summary, _ := json.Marshal(map[string]any{
		"new": res.New, "changed": res.Changed, "removed": res.Removed,
		"quantity_changes": res.QuantityChanges, "issues": issues,
	})
	finished := now()
	if err := q.FinishImportRun(ctx, db.FinishImportRunParams{
		ID: res.RunID, Status: res.Status, IssueCount: int32(res.Issues), Summary: summary, FinishedAt: &finished}); err != nil {
		return Result{}, err
	}
	return res, tx.Commit(ctx)
}

type prevItem struct {
	id      int64
	active  bool
	rawID   *int64
	rawHash string
	item    connector.SupplierItem
}

type runState struct {
	ctx        context.Context
	q          *db.Queries
	runID      int64
	sourceID   string
	rules      []rules.Rule
	priceRules []pricing.Rule
	policies   catalog.UsagePolicies
	res        *Result
	// newItems collects the articles first seen in this run; they are
	// decided per model once all articles are read.
	newItems []newItem
}

type newItem struct {
	id   int64
	item connector.SupplierItem
}

// modelKey identifies a model within its source. An article without a name
// cannot be grouped and is a model of its own.
func modelKey(sourceID string, it connector.SupplierItem) string {
	m := catalog.ModelOf(it)
	if m.Name == "" {
		return strings.Join([]string{sourceID, "sku", it.SKU}, "\x1f")
	}
	return strings.Join([]string{sourceID, m.Brand, m.Name, fmt.Sprint(m.ModelYear)}, "\x1f")
}

// decideNewModels creates one new_product action per model for the articles
// first seen in this run (docs/decisions/0022). A pending action for
// the same model is superseded and its articles carried over, so there is
// never more than one open task per model.
func (s *runState) decideNewModels() error {
	type model struct {
		key      string
		title    string
		brand    string
		category string
		ids      []int64
	}
	byKey := map[string]*model{}
	var order []string
	for _, n := range s.newItems {
		key := modelKey(s.sourceID, n.item)
		m, ok := byKey[key]
		if !ok {
			title := catalog.ModelOf(n.item).Title()
			if title == "" {
				title = n.item.SKU
			}
			m = &model{key: key, title: title, brand: n.item.Brand, category: n.item.Category}
			byKey[key] = m
			order = append(order, key)
		}
		if n.item.Category != m.category {
			m.category = "" // mixed categories: only rules without category apply
		}
		m.ids = append(m.ids, n.id)
	}
	sort.Strings(order)

	for _, key := range order {
		m := byKey[key]
		d := rules.Evaluate(s.rules, rules.Action{
			Type: rules.NewProduct, SourceID: s.sourceID, Brand: m.brand, Category: m.category,
			Facts: map[rules.Fact]float64{}})

		known, err := s.q.ModelWasAccepted(s.ctx, key)
		if err != nil {
			return err
		}
		superseded, err := s.q.SupersedePendingModelActions(s.ctx, key)
		if err != nil {
			return err
		}
		ids := m.ids
		if len(superseded) > 0 {
			if err := s.q.DismissTasksOfActions(s.ctx, superseded); err != nil {
				return err
			}
			carried, err := s.q.ListActionItemIDs(s.ctx, superseded)
			if err != nil {
				return err
			}
			ids = append(carried, ids...)
		}

		params := db.InsertActionParams{
			Type: string(rules.NewProduct), Status: "pending_approval", NewValue: &m.title, Facts: []byte("{}"),
			DecisionReason: d.Reason, ImportRunID: &s.runID, ModelKey: key,
		}
		if d.RuleID != 0 {
			params.RuleID = &d.RuleID
		}
		if d.Outcome == rules.Auto {
			at, by := time.Now(), "rule"
			params.Status, params.DecidedAt, params.DecidedBy = "auto_applied", &at, &by
		}
		actionID, err := s.q.InsertAction(s.ctx, params)
		if err != nil {
			return err
		}
		if err := s.q.InsertActionItems(s.ctx, db.InsertActionItemsParams{ActionID: actionID, ItemIds: ids}); err != nil {
			return err
		}
		if d.Outcome == rules.Auto {
			s.res.AutoApplied++
			continue
		}
		s.res.PendingApproval++
		kind := "Neues Modell prüfen"
		if known {
			kind = "Neue Varianten prüfen"
		}
		title := fmt.Sprintf("%s: %s, %s", kind, m.title, plural(len(ids), "Variante", "Varianten"))
		if err := s.task("approve_action", 2, title, "", &actionID, nil); err != nil {
			return err
		}
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// upsert writes one article and everything that follows from its changes.
func (s *runState) upsert(rec connector.RawRecord, item connector.SupplierItem, prev *prevItem) error {
	hash := rec.Hash()
	var rawID *int64
	if prev != nil {
		rawID = prev.rawID
	}
	if prev == nil || prev.rawHash != hash {
		fields, _ := json.Marshal(rec.Fields)
		id, err := s.q.InsertRawRecord(s.ctx, db.InsertRawRecordParams{
			SourceID: s.sourceID, Key: rec.Key, ContentHash: hash, Fields: fields, FirstImportRunID: s.runID})
		if err != nil {
			return err
		}
		rawID = &id
	}

	images, _ := json.Marshal(nonNil(item.ImageURLs))
	attrs, _ := json.Marshal(nonNilMap(item.Attributes))
	avail, _ := json.Marshal(item.Availability)
	var year *int32
	if item.ModelYear != 0 {
		y := int32(item.ModelYear)
		year = &y
	}
	params := db.InsertSupplierItemParams{
		SourceID: s.sourceID, Sku: item.SKU, Ean: item.EAN, Mpn: item.MPN, Brand: item.Brand, Name: item.Name,
		ModelYear: year, Category: item.Category, Color: item.Color, FrameSize: item.FrameSize,
		Description: item.Description, ImageUrls: images, CostCents: nullInt(item.CostCents),
		RrpCents: nullInt(item.RRPCents), Attributes: attrs, Availability: avail,
		AvailabilityStatus: string(item.Availability.Status), RawRecordID: rawID, LastImportRunID: &s.runID,
	}

	if prev == nil {
		id, err := s.q.InsertSupplierItem(s.ctx, params)
		if err != nil {
			return err
		}
		s.res.New++
		s.newItems = append(s.newItems, newItem{id: id, item: item})
		return nil
	}

	if err := s.q.UpdateSupplierItem(s.ctx, db.UpdateSupplierItemParams(params)); err != nil {
		return err
	}
	if !prev.active {
		if err := s.fieldChange(prev.id, "active", "false", "true"); err != nil {
			return err
		}
	}
	if !sameQuantity(prev.item.Availability.Quantity, item.Availability.Quantity) {
		s.res.QuantityChanges++
	}
	proposals := actions.Propose(&prev.item, item)
	if len(proposals) > 0 {
		s.res.Changed++
	}
	for _, p := range proposals {
		if err := s.fieldChange(prev.id, p.Field, p.Old, p.New); err != nil {
			return err
		}
		if err := s.decide(p, prev.id, item, &prev.item); err != nil {
			return err
		}
	}
	return nil
}

// decide lets the rules decide about one proposal and records the action.
// A pending action gets a task; an older pending action for the same field
// is superseded so that tasks do not pile up.
func (s *runState) decide(p actions.Proposal, itemID int64, item connector.SupplierItem, prev *connector.SupplierItem) error {
	// Changes of a field group the source's usage policy ignores create no
	// action; the supplier view and its history are updated regardless.
	if p.Field != "" && s.policies.For(s.sourceID, item.Brand, catalog.GroupOfField(p.Field)) == catalog.PolicyIgnore {
		s.res.Ignored++
		return nil
	}
	superseded, err := s.q.SupersedePendingActions(s.ctx, db.SupersedePendingActionsParams{
		SupplierItemID: &itemID, Type: string(p.Type), Field: p.Field})
	if err != nil {
		return err
	}
	if len(superseded) > 0 {
		ids := make([]int64, len(superseded))
		for i, sup := range superseded {
			ids[i] = sup.ID
		}
		if err := s.q.DismissTasksOfActions(s.ctx, ids); err != nil {
			return err
		}
	}

	facts := map[rules.Fact]float64{}
	if p.Type == rules.PriceChange && prev != nil {
		facts = s.priceFacts(*prev, item)
	}
	// A changed purchase price proposes the same amount on the own sales
	// price (docs/decisions/0018). An undecided earlier change is carried
	// over, so the proposal covers everything since the last decision.
	var variantID, basePrice, proposedPrice *int64
	if p.Type == rules.PriceChange && p.Field == "cost_cents" && prev != nil {
		delta := item.CostCents - prev.CostCents
		for _, sup := range superseded {
			if sup.BasePriceCents != nil && sup.ProposedPriceCents != nil {
				delta += *sup.ProposedPriceCents - *sup.BasePriceCents
				if sup.OldValue != nil {
					p.Old = *sup.OldValue
				}
			}
		}
		facts[rules.CostChangeCents] = float64(delta)
		if prev.CostCents > 0 && item.CostCents > 0 {
			v, err := s.q.GetPricedVariantOfSupplierItem(s.ctx, itemID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err == nil && *v.PriceCents+delta > 0 {
				proposed := *v.PriceCents + delta
				variantID, basePrice, proposedPrice = &v.ID, v.PriceCents, &proposed
			}
		}
	}
	d := rules.Evaluate(s.rules, rules.Action{
		Type: p.Type, SourceID: s.sourceID, Brand: item.Brand, Category: item.Category, Facts: facts})

	factsJSON, _ := json.Marshal(facts)
	params := db.InsertActionParams{
		Type: string(p.Type), Status: "pending_approval", SupplierItemID: &itemID, Field: p.Field,
		OldValue: &p.Old, NewValue: &p.New, Facts: factsJSON, DecisionReason: d.Reason, ImportRunID: &s.runID,
		VariantID: variantID, BasePriceCents: basePrice, ProposedPriceCents: proposedPrice,
	}
	if d.RuleID != 0 {
		params.RuleID = &d.RuleID
	}
	if d.Outcome == rules.Auto {
		at, by := time.Now(), "rule"
		params.Status, params.DecidedAt, params.DecidedBy = "auto_applied", &at, &by
	}
	actionID, err := s.q.InsertAction(s.ctx, params)
	if err != nil {
		return err
	}
	if d.Outcome == rules.Auto {
		s.res.AutoApplied++
		if proposedPrice != nil {
			return catalog.ShiftVariantPrice(s.ctx, s.q, *variantID, *proposedPrice-*basePrice, "rule", actionID, "",
				"Einkaufspreis des Lieferanten geändert")
		}
		return catalog.ApplyItemChange(s.ctx, s.q, actionID, itemID, p.Field, p.New, "rule", "")
	}
	s.res.PendingApproval++
	title, prio := taskTitle(p, item)
	return s.task("approve_action", prio, title, "", &actionID, &itemID)
}

// priceFacts computes what price rules test: how the sales price would
// change and the margin of the new price. Without a pricing rule for the
// article there are no facts, so conditions on them do not match.
func (s *runState) priceFacts(prev, cur connector.SupplierItem) map[rules.Fact]float64 {
	cfg, ok := pricing.Pick(s.priceRules, cur.Brand, cur.Category)
	if !ok {
		return map[rules.Fact]float64{}
	}
	oldP, err1 := pricing.Calculate(prev.CostCents, prev.RRPCents, cfg)
	newP, err2 := pricing.Calculate(cur.CostCents, cur.RRPCents, cfg)
	if err1 != nil || err2 != nil {
		return map[rules.Fact]float64{}
	}
	change := pricing.ChangePct(oldP.PriceCents, newP.PriceCents)
	return map[rules.Fact]float64{
		rules.PriceChangePct:    change,
		rules.PriceChangeAbsPct: math.Abs(change),
		rules.MarginPct:         newP.MarginPct,
	}
}

func (s *runState) fieldChange(itemID int64, field, old, new string) error {
	return s.q.InsertImportFieldChange(s.ctx, db.InsertImportFieldChangeParams{
		EntityID: itemID, Field: field, OldValue: &old, NewValue: &new, ImportRunID: &s.runID})
}

func (s *runState) task(kind string, prio int, title, detail string, actionID, itemID *int64) error {
	return s.q.InsertTask(s.ctx, db.InsertTaskParams{
		Kind: kind, Title: title, Detail: detail, Priority: int16(prio),
		ActionID: actionID, SupplierItemID: itemID, ImportRunID: &s.runID})
}

func taskTitle(p actions.Proposal, item connector.SupplierItem) (string, int) {
	name := strings.TrimSpace(item.Brand + " " + item.Name)
	if name == "" {
		name = p.SKU
	}
	switch p.Type {
	case rules.NewProduct:
		return "Neues Produkt prüfen: " + name, 2
	case rules.PriceChange:
		return "Preisänderung prüfen: " + name, 1
	case rules.AvailabilityChange:
		return "Verfügbarkeit prüfen: " + name, 2
	case rules.DescriptionChange:
		return "Textänderung prüfen: " + name, 3
	case rules.ImagesChange:
		return "Bildänderung prüfen: " + name, 3
	case rules.Deactivation:
		return "Artikel fehlt im Lieferantenfeed: " + name, 1
	}
	return "Änderung prüfen (" + p.Field + "): " + name, 3
}

func loadRules(ctx context.Context, q *db.Queries) ([]rules.Rule, error) {
	rows, err := q.ListEnabledRules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]rules.Rule, 0, len(rows))
	for _, row := range rows {
		r := rules.Rule{
			ID: row.ID, Name: row.Name, Enabled: row.Enabled, ActionType: rules.ActionType(row.ActionType),
			Scope:   rules.Scope{SourceID: row.ScopeSourceID, Brand: row.ScopeBrand, Category: row.ScopeCategory},
			Outcome: rules.Outcome(row.Outcome), Guard: row.Guard,
		}
		if err := json.Unmarshal(row.Conditions, &r.Conditions); err != nil {
			return nil, fmt.Errorf("importer: conditions of rule %d: %w", row.ID, err)
		}
		out = append(out, r)
	}
	return out, nil
}

func loadPricingRules(ctx context.Context, q *db.Queries) ([]pricing.Rule, error) {
	rows, err := q.ListPricingRules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]pricing.Rule, 0, len(rows))
	for _, row := range rows {
		r := pricing.Rule{Brand: row.ScopeBrand, Category: row.ScopeCategory}
		if err := json.Unmarshal(row.Config, &r.Config); err != nil {
			return nil, fmt.Errorf("importer: pricing rule: %w", err)
		}
		out = append(out, r)
	}
	return out, nil
}

func loadItems(ctx context.Context, q *db.Queries, sourceID string) (map[string]*prevItem, error) {
	rows, err := q.ListSupplierItems(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*prevItem, len(rows))
	for _, row := range rows {
		p := &prevItem{id: row.ID, active: row.Active, rawID: row.RawRecordID, rawHash: row.RawHash}
		p.item = connector.SupplierItem{
			SourceID: sourceID, SKU: row.Sku, EAN: row.Ean, MPN: row.Mpn, Brand: row.Brand, Name: row.Name,
			Category: row.Category, Color: row.Color, FrameSize: row.FrameSize, Description: row.Description,
		}
		it := &p.item
		if row.ModelYear != nil {
			it.ModelYear = int(*row.ModelYear)
		}
		if row.CostCents != nil {
			it.CostCents = *row.CostCents
		}
		if row.RrpCents != nil {
			it.RRPCents = *row.RrpCents
		}
		if err := errors.Join(json.Unmarshal(row.ImageUrls, &it.ImageURLs), json.Unmarshal(row.Attributes, &it.Attributes),
			json.Unmarshal(row.Availability, &it.Availability)); err != nil {
			return nil, fmt.Errorf("importer: stored article %s: %w", it.SKU, err)
		}
		if len(it.ImageURLs) == 0 {
			it.ImageURLs = nil
		}
		if len(it.Attributes) == 0 {
			it.Attributes = nil
		}
		out[it.SKU] = p
	}
	return out, nil
}

func storeOriginal(dir, sourceID, hash string, at time.Time, data []byte) (string, error) {
	if dir == "" {
		return "", errors.New("importer: RawDir is required")
	}
	dir = filepath.Join(dir, sourceID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	path := filepath.Join(dir, at.UTC().Format("20060102T150405Z")+"-"+hash[:12])
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return "", err
	}
	return path, nil
}

func sameQuantity(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func nullInt(n int64) *int64 {
	if n == 0 {
		return nil
	}
	return &n
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
