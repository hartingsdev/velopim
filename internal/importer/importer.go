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
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/actions"
	"velopim/internal/availability"
	"velopim/internal/connector"
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

	var mappingJSON []byte
	if err := tx.QueryRow(ctx, `SELECT availability_mapping FROM source WHERE id = $1`, sourceID).Scan(&mappingJSON); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Result{}, fmt.Errorf("importer: source %q is not configured", sourceID)
		}
		return Result{}, err
	}
	var mapping []availability.MappingRule
	if err := json.Unmarshal(mappingJSON, &mapping); err != nil {
		return Result{}, fmt.Errorf("importer: availability mapping of %q: %w", sourceID, err)
	}
	ruleSet, err := loadRules(ctx, tx)
	if err != nil {
		return Result{}, err
	}
	priceRules, err := loadPricingRules(ctx, tx)
	if err != nil {
		return Result{}, err
	}
	prev, err := loadItems(ctx, tx, sourceID)
	if err != nil {
		return Result{}, err
	}

	res := Result{Status: "done", Records: len(parsed.Records)}
	if err := tx.QueryRow(ctx,
		`INSERT INTO import_run (source_id, status, file_path, file_sha256, record_count, started_at)
		 VALUES ($1, 'running', $2, $3, $4, $5) RETURNING id`,
		sourceID, filePath, fileHash, len(parsed.Records), now()).Scan(&res.RunID); err != nil {
		return Result{}, err
	}
	run := &runState{ctx: ctx, tx: tx, runID: res.RunID, sourceID: sourceID, rules: ruleSet, priceRules: priceRules, res: &res}

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
			if _, err := tx.Exec(ctx, `UPDATE supplier_item SET active = false, updated_at = now() WHERE id = $1`, it.id); err != nil {
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
	if _, err := tx.Exec(ctx,
		`UPDATE import_run SET status = $2, issue_count = $3, summary = $4, finished_at = $5 WHERE id = $1`,
		res.RunID, res.Status, res.Issues, summary, now()); err != nil {
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
	tx         pgx.Tx
	runID      int64
	sourceID   string
	rules      []rules.Rule
	priceRules []pricingRule
	res        *Result
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
		var id int64
		if err := s.tx.QueryRow(s.ctx,
			`INSERT INTO raw_record (source_id, key, content_hash, fields, first_import_run_id)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			s.sourceID, rec.Key, hash, fields, s.runID).Scan(&id); err != nil {
			return err
		}
		rawID = &id
	}

	images, _ := json.Marshal(nonNil(item.ImageURLs))
	attrs, _ := json.Marshal(nonNilMap(item.Attributes))
	avail, _ := json.Marshal(item.Availability)
	args := []any{s.sourceID, item.SKU, item.EAN, item.MPN, item.Brand, item.Name, nullInt(int64(item.ModelYear)),
		item.Category, item.Color, item.FrameSize, item.Description, images,
		nullInt(item.CostCents), nullInt(item.RRPCents), attrs, avail, string(item.Availability.Status), rawID, s.runID}

	if prev == nil {
		var id int64
		if err := s.tx.QueryRow(s.ctx,
			`INSERT INTO supplier_item (source_id, sku, ean, mpn, brand, name, model_year, category, color, frame_size,
			   description, image_urls, cost_cents, rrp_cents, attributes, availability, availability_status,
			   raw_record_id, last_import_run_id)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) RETURNING id`,
			args...).Scan(&id); err != nil {
			return err
		}
		s.res.New++
		return s.decide(actions.Proposal{Type: rules.NewProduct, SKU: item.SKU}, id, item, nil)
	}

	if _, err := s.tx.Exec(s.ctx,
		`UPDATE supplier_item SET ean=$3, mpn=$4, brand=$5, name=$6, model_year=$7, category=$8, color=$9,
		   frame_size=$10, description=$11, image_urls=$12, cost_cents=$13, rrp_cents=$14, attributes=$15,
		   availability=$16, availability_status=$17, raw_record_id=$18, last_import_run_id=$19,
		   active=true, updated_at=now()
		 WHERE source_id=$1 AND sku=$2`, args...); err != nil {
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
	facts := map[rules.Fact]float64{}
	if p.Type == rules.PriceChange && prev != nil {
		facts = s.priceFacts(*prev, item)
	}
	d := rules.Evaluate(s.rules, rules.Action{
		Type: p.Type, SourceID: s.sourceID, Brand: item.Brand, Category: item.Category, Facts: facts})

	rows, err := s.tx.Query(s.ctx,
		`UPDATE action SET status = 'superseded', decided_at = now(), decided_by = 'system'
		 WHERE supplier_item_id = $1 AND type = $2 AND field = $3 AND status = 'pending_approval' RETURNING id`,
		itemID, string(p.Type), p.Field)
	if err != nil {
		return err
	}
	superseded, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return err
	}
	if len(superseded) > 0 {
		if _, err := s.tx.Exec(s.ctx,
			`UPDATE task SET status = 'dismissed', closed_at = now(), closed_by = 'system',
			   note = 'Durch neuere Änderung ersetzt'
			 WHERE action_id = ANY($1) AND status = 'open'`, superseded); err != nil {
			return err
		}
	}

	status := "pending_approval"
	if d.Outcome == rules.Auto {
		status = "auto_applied"
	}
	factsJSON, _ := json.Marshal(facts)
	var ruleID *int64
	if d.RuleID != 0 {
		ruleID = &d.RuleID
	}
	var actionID int64
	if err := s.tx.QueryRow(s.ctx,
		`INSERT INTO action (type, status, supplier_item_id, field, old_value, new_value, facts, rule_id,
		   decision_reason, import_run_id, decided_at, decided_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,
		   CASE WHEN $2 = 'auto_applied' THEN now() END, CASE WHEN $2 = 'auto_applied' THEN 'rule' END)
		 RETURNING id`,
		string(p.Type), status, itemID, p.Field, p.Old, p.New, factsJSON, ruleID, d.Reason, s.runID).Scan(&actionID); err != nil {
		return err
	}
	if d.Outcome == rules.Auto {
		s.res.AutoApplied++
		return nil
	}
	s.res.PendingApproval++
	title, prio := taskTitle(p, item)
	return s.task("approve_action", prio, title, "", &actionID, &itemID)
}

// priceFacts computes what price rules test: how the sales price would
// change and the margin of the new price. Without a pricing rule for the
// article there are no facts, so conditions on them do not match.
func (s *runState) priceFacts(prev, cur connector.SupplierItem) map[rules.Fact]float64 {
	cfg, ok := pickPricing(s.priceRules, cur.Brand, cur.Category)
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
	_, err := s.tx.Exec(s.ctx,
		`INSERT INTO field_change (entity_type, entity_id, field, old_value, new_value, origin, import_run_id)
		 VALUES ('supplier_item', $1, $2, $3, $4, 'import', $5)`, itemID, field, old, new, s.runID)
	return err
}

func (s *runState) task(kind string, prio int, title, detail string, actionID, itemID *int64) error {
	_, err := s.tx.Exec(s.ctx,
		`INSERT INTO task (kind, title, detail, priority, action_id, supplier_item_id, import_run_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`, kind, title, detail, prio, actionID, itemID, s.runID)
	return err
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

type pricingRule struct {
	brand, category string
	cfg             pricing.Config
}

// pickPricing returns the most specific pricing rule: brand and category,
// then brand, then category, then the global rule.
func pickPricing(rs []pricingRule, brand, category string) (pricing.Config, bool) {
	best, bestScore := pricing.Config{}, -1
	for _, r := range rs {
		if r.brand != "" && r.brand != brand || r.category != "" && r.category != category {
			continue
		}
		score := 0
		if r.brand != "" {
			score += 2
		}
		if r.category != "" {
			score++
		}
		if score > bestScore {
			best, bestScore = r.cfg, score
		}
	}
	return best, bestScore >= 0
}

func loadRules(ctx context.Context, tx pgx.Tx) ([]rules.Rule, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, name, enabled, action_type, scope_source_id, scope_brand, scope_category, conditions, outcome, guard
		 FROM rule WHERE enabled`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (rules.Rule, error) {
		var r rules.Rule
		var conds []byte
		var actionType, outcome string
		if err := row.Scan(&r.ID, &r.Name, &r.Enabled, &actionType, &r.Scope.SourceID, &r.Scope.Brand,
			&r.Scope.Category, &conds, &outcome, &r.Guard); err != nil {
			return r, err
		}
		r.ActionType, r.Outcome = rules.ActionType(actionType), rules.Outcome(outcome)
		return r, json.Unmarshal(conds, &r.Conditions)
	})
}

func loadPricingRules(ctx context.Context, tx pgx.Tx) ([]pricingRule, error) {
	rows, err := tx.Query(ctx, `SELECT scope_brand, scope_category, config FROM pricing_rule`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (pricingRule, error) {
		var r pricingRule
		var cfg []byte
		if err := row.Scan(&r.brand, &r.category, &cfg); err != nil {
			return r, err
		}
		return r, json.Unmarshal(cfg, &r.cfg)
	})
}

func loadItems(ctx context.Context, tx pgx.Tx, sourceID string) (map[string]*prevItem, error) {
	rows, err := tx.Query(ctx,
		`SELECT s.id, s.active, s.raw_record_id, coalesce(r.content_hash, ''), s.sku, s.ean, s.mpn, s.brand, s.name,
		   coalesce(s.model_year, 0), s.category, s.color, s.frame_size, s.description, s.image_urls,
		   coalesce(s.cost_cents, 0), coalesce(s.rrp_cents, 0), s.attributes, s.availability
		 FROM supplier_item s LEFT JOIN raw_record r ON r.id = s.raw_record_id
		 WHERE s.source_id = $1`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*prevItem{}
	for rows.Next() {
		p := &prevItem{}
		it := &p.item
		var images, attrs, avail []byte
		if err := rows.Scan(&p.id, &p.active, &p.rawID, &p.rawHash, &it.SKU, &it.EAN, &it.MPN, &it.Brand, &it.Name,
			&it.ModelYear, &it.Category, &it.Color, &it.FrameSize, &it.Description, &images,
			&it.CostCents, &it.RRPCents, &attrs, &avail); err != nil {
			return nil, err
		}
		it.SourceID = sourceID
		if err := errors.Join(json.Unmarshal(images, &it.ImageURLs), json.Unmarshal(attrs, &it.Attributes),
			json.Unmarshal(avail, &it.Availability)); err != nil {
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
	return out, rows.Err()
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
