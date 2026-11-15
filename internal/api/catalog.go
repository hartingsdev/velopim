package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"velopim/internal/catalog"
	"velopim/internal/db"
	"velopim/internal/pricing"
)

// addModelDetails fills the variants of new-model tasks, with the price the
// pricing rules calculate, and the cut of models that already have products.
func (s *server) addModelDetails(ctx context.Context, q *db.Queries, rows []db.ListTasksRow, out []Task) error {
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

// Product is one product of the own catalog with its variants. Products cut
// from the same model share ModelKey and ModelName.
type Product struct {
	ID          int64     `json:"id"`
	Brand       string    `json:"brand"`
	Name        string    `json:"name"`
	ModelYear   *int32    `json:"model_year"`
	Category    string    `json:"category"`
	Status      string    `json:"status"`
	OptionNames []string  `json:"option_names"`
	CreatedAt   time.Time `json:"created_at"`
	ModelKey    string    `json:"model_key"`
	ModelName   string    `json:"model_name"`
	// Split holds the values this product was separated by for the shop.
	Split       []catalog.AxisValue `json:"split"`
	Description string              `json:"description_html"`
	ImageURLs   []string            `json:"image_urls"`
	// Wanted says the product is selected for the store.
	Wanted   bool      `json:"wanted"`
	Variants []Variant `json:"variants"`
}

// ProductList is the response of GET /api/products.
type ProductList struct {
	Products []Product `json:"products"`
	Total    int64     `json:"total"`
	Unpriced int64     `json:"unpriced"`
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
	query := r.URL.Query()
	status := query.Get("status")
	if status != "" && status != "draft" && status != "active" && status != "archived" {
		writeError(w, http.StatusBadRequest, "unknown status")
		return
	}
	rows, err := q.ListProducts(r.Context(), db.ListProductsParams{Status: status, Unpriced: query.Get("unpriced") == "true",
		Q: strings.TrimSpace(query.Get("q")), MaxRows: listLimit})
	if err != nil {
		internalError(w, err)
		return
	}
	counts, err := q.CountProducts(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	products, err := buildProducts(r.Context(), q, rows)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ProductList{Products: products, Total: counts.Total, Unpriced: counts.Unpriced})
}

// buildProducts turns product rows into API products with their variants.
func buildProducts(ctx context.Context, q *db.Queries, rows []db.ListProductsRow) ([]Product, error) {
	out := make([]Product, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	index := map[int64]int{}
	for i, row := range rows {
		p := Product{ID: row.ID, Brand: row.Brand, Name: row.Name, ModelYear: row.ModelYear, Category: row.Category,
			Status: row.Status, OptionNames: []string{}, CreatedAt: row.CreatedAt, ModelKey: row.ModelKey,
			Split: []catalog.AxisValue{}, Variants: []Variant{}, Description: row.DescriptionHtml,
			ImageURLs: []string{}, Wanted: row.Wanted}
		_ = json.Unmarshal(row.OptionNames, &p.OptionNames)
		_ = json.Unmarshal(row.SplitValues, &p.Split)
		_ = json.Unmarshal(row.ImageUrls, &p.ImageURLs)
		// The product name is the model name followed by the split values.
		p.ModelName = p.Name
		for i := len(p.Split) - 1; i >= 0; i-- {
			if v := p.Split[i].Value; v != "" {
				p.ModelName = strings.TrimSuffix(p.ModelName, " "+v)
			}
		}
		out = append(out, p)
		ids = append(ids, row.ID)
		index[row.ID] = i
	}
	variants, err := q.ListVariantsOfProducts(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, v := range variants {
		item := Variant{ID: v.ID, SKU: v.Sku, EAN: v.Ean, OptionValues: []string{}, PriceCents: v.PriceCents,
			CompareAtCents: v.CompareAtCents, SupplierSKU: v.SupplierSku, SourceID: v.SourceID, CostCents: v.CostCents,
			RRPCents: v.RrpCents, Availability: v.AvailabilityStatus}
		_ = json.Unmarshal(v.OptionValues, &item.OptionValues)
		p := &out[index[v.ProductID]]
		p.Variants = append(p.Variants, item)
	}
	return out, nil
}

// SupplierItem is one article of a supplier as found by the search.
type SupplierItem struct {
	ID           int64   `json:"id"`
	SourceID     string  `json:"source_id"`
	SKU          string  `json:"sku"`
	EAN          string  `json:"ean"`
	Brand        string  `json:"brand"`
	Name         string  `json:"name"`
	ModelYear    *int32  `json:"model_year"`
	Color        string  `json:"color"`
	FrameSize    string  `json:"frame_size"`
	CostCents    *int64  `json:"cost_cents"`
	RRPCents     *int64  `json:"rrp_cents"`
	Availability string  `json:"availability"`
	Active       bool    `json:"active"`
	OwnSKU       *string `json:"own_sku"`
	ProductName  *string `json:"product_name"`
}

// searchSupplierItems finds supplier articles by article number, EAN,
// manufacturer number, brand, name or color. An empty query finds nothing.
func (s *server) searchSupplierItems(w http.ResponseWriter, r *http.Request) {
	out := []SupplierItem{}
	if search := strings.TrimSpace(r.URL.Query().Get("q")); search != "" {
		rows, err := db.New(s.pool).SearchSupplierItems(r.Context(), db.SearchSupplierItemsParams{Q: search, MaxRows: 50})
		if err != nil {
			internalError(w, err)
			return
		}
		for _, row := range rows {
			out = append(out, SupplierItem{ID: row.ID, SourceID: row.SourceID, SKU: row.Sku, EAN: row.Ean, Brand: row.Brand,
				Name: row.Name, ModelYear: row.ModelYear, Color: row.Color, FrameSize: row.FrameSize, CostCents: row.CostCents,
				RRPCents: row.RrpCents, Availability: row.AvailabilityStatus, Active: row.Active, OwnSKU: row.OwnSku,
				ProductName: row.ProductName})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
