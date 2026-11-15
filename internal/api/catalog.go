package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"velopim/internal/db"
	"velopim/internal/pricing"
)

// addModelDetails fills the variants of new-model tasks, with the price the
// pricing rules calculate, and the cut of models that already have products.
func (s *server) addModelDetails(ctx context.Context, q *db.Queries, rows []db.ListOpenTasksRow, out []Task) error {
	var actionIDs []int64
	var modelKeys []string
	for _, row := range rows {
		if row.ActionID != nil && row.ModelKey != nil && *row.ModelKey != "" {
			actionIDs = append(actionIDs, *row.ActionID)
			modelKeys = append(modelKeys, *row.ModelKey)
		}
	}
	if len(actionIDs) == 0 {
		return nil
	}
	items, err := q.ListActionItems(ctx, actionIDs)
	if err != nil {
		return err
	}
	rules, err := loadPricingRules(ctx, q)
	if err != nil {
		return err
	}
	byAction := map[int64][]TaskItem{}
	for _, it := range items {
		v := TaskItem{SourceID: it.SourceID, SKU: it.Sku, Brand: it.Brand, Name: it.Name, ModelYear: it.ModelYear,
			Color: it.Color, FrameSize: it.FrameSize, Category: it.Category, CostCents: it.CostCents, RRPCents: it.RrpCents,
			Attributes: map[string]string{}}
		_ = json.Unmarshal(it.Attributes, &v.Attributes)
		if cfg, ok := pricing.Pick(rules, it.Brand, it.Category); ok && it.CostCents != nil {
			var rrp int64
			if it.RrpCents != nil {
				rrp = *it.RrpCents
			}
			if res, err := pricing.Calculate(*it.CostCents, rrp, cfg); err == nil {
				v.PriceCents = &res.PriceCents
			}
		}
		byAction[it.ActionID] = append(byAction[it.ActionID], v)
	}
	splits, err := q.ListModelSplits(ctx, modelKeys)
	if err != nil {
		return err
	}
	splitByModel := map[string][]string{}
	for _, sp := range splits {
		axes := []string{}
		_ = json.Unmarshal(sp.SplitBy, &axes)
		splitByModel[sp.ModelKey] = axes
	}
	for i, row := range rows {
		if row.ActionID == nil || row.ModelKey == nil {
			continue
		}
		if v := byAction[*row.ActionID]; v != nil {
			out[i].Variants = v
		}
		out[i].LockedSplitBy = splitByModel[*row.ModelKey]
	}
	return nil
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
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// PricingRule is one calculation; empty brand or category means "all".
type PricingRule struct {
	ID       int64          `json:"id"`
	Brand    string         `json:"brand"`
	Category string         `json:"category"`
	Config   pricing.Config `json:"config"`
}

func (s *server) listPricingRules(w http.ResponseWriter, r *http.Request) {
	rows, err := db.New(s.pool).ListAllPricingRules(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]PricingRule, 0, len(rows))
	for _, row := range rows {
		pr := PricingRule{ID: row.ID, Brand: row.ScopeBrand, Category: row.ScopeCategory}
		if err := json.Unmarshal(row.Config, &pr.Config); err != nil {
			internalError(w, err)
			return
		}
		out = append(out, pr)
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": out})
}

// putPricingRule creates the rule for a brand and category or replaces it.
func (s *server) putPricingRule(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Brand    string         `json:"brand"`
		Category string         `json:"category"`
		Config   pricing.Config `json:"config"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := in.Config.Validate(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	cfg, _ := json.Marshal(in.Config)
	id, err := db.New(s.pool).UpsertPricingRule(r.Context(), db.UpsertPricingRuleParams{
		ScopeBrand: in.Brand, ScopeCategory: in.Category, Config: cfg})
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, PricingRule{ID: id, Brand: in.Brand, Category: in.Category, Config: in.Config})
}

func (s *server) deletePricingRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	n, err := db.New(s.pool).DeletePricingRule(r.Context(), id)
	if err != nil {
		internalError(w, err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Product is one product of the own catalog with its variants.
type Product struct {
	ID          int64     `json:"id"`
	Brand       string    `json:"brand"`
	Name        string    `json:"name"`
	ModelYear   *int32    `json:"model_year"`
	Category    string    `json:"category"`
	Status      string    `json:"status"`
	OptionNames []string  `json:"option_names"`
	CreatedAt   time.Time `json:"created_at"`
	Variants    []Variant `json:"variants"`
}

// Variant is one variant with the preferred supplier's article behind it.
type Variant struct {
	ID             int64    `json:"id"`
	SKU            string   `json:"sku"`
	EAN            string   `json:"ean"`
	OptionValues   []string `json:"option_values"`
	PriceCents     *int64   `json:"price_cents"`
	CompareAtCents *int64   `json:"compare_at_cents"`
	SupplierSKU    *string  `json:"supplier_sku"`
	SourceID       *string  `json:"source_id"`
	CostCents      *int64   `json:"cost_cents"`
	RRPCents       *int64   `json:"rrp_cents"`
	Availability   *string  `json:"availability"`
}

func (s *server) listProducts(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.pool)
	rows, err := q.ListProducts(r.Context(), listLimit)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]Product, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	index := map[int64]int{}
	for i, row := range rows {
		p := Product{ID: row.ID, Brand: row.Brand, Name: row.Name, ModelYear: row.ModelYear, Category: row.Category,
			Status: row.Status, OptionNames: []string{}, CreatedAt: row.CreatedAt, Variants: []Variant{}}
		_ = json.Unmarshal(row.OptionNames, &p.OptionNames)
		out = append(out, p)
		ids = append(ids, row.ID)
		index[row.ID] = i
	}
	variants, err := q.ListVariantsOfProducts(r.Context(), ids)
	if err != nil {
		internalError(w, err)
		return
	}
	for _, v := range variants {
		item := Variant{ID: v.ID, SKU: v.Sku, EAN: v.Ean, OptionValues: []string{}, PriceCents: v.PriceCents,
			CompareAtCents: v.CompareAtCents, SupplierSKU: v.SupplierSku, SourceID: v.SourceID, CostCents: v.CostCents,
			RRPCents: v.RrpCents, Availability: v.AvailabilityStatus}
		_ = json.Unmarshal(v.OptionValues, &item.OptionValues)
		p := &out[index[v.ProductID]]
		p.Variants = append(p.Variants, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": out})
}
