package api

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"velopim/internal/availability"
	"velopim/internal/db"
	"velopim/internal/shopify"
)

// SyncPlan is the response of GET /api/sync-plan: what a sync would send to
// Shopify. Nothing is sent; there is no sync client yet.
type SyncPlan struct {
	// StoreConfigured reports whether Shopify credentials are set.
	StoreConfigured bool `json:"store_configured"`
	// WriteEnabled mirrors SHOPIFY_WRITE_ENABLED.
	WriteEnabled bool          `json:"write_enabled"`
	Products     []PlanProduct `json:"products"`
}

// PlanProduct is the planned sync of one product.
type PlanProduct struct {
	ProductID  int64           `json:"product_id"`
	Name       string          `json:"name"`
	InShopify  bool            `json:"in_shopify"`
	LastSynced *time.Time      `json:"last_synced_at"`
	LastError  *string         `json:"last_error"`
	Ops        []shopify.Op    `json:"ops"`
	Issues     []string        `json:"issues"` // why Shopify would refuse the product
	Projection shopify.Product `json:"projection"`
}

func (s *server) syncPlan(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.pool)
	rows, err := q.ListWantedProducts(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	variants, err := q.ListVariantsForProjection(r.Context(), ids)
	if err != nil {
		internalError(w, err)
		return
	}
	byProduct := map[int64][]shopify.CatalogVariant{}
	for _, v := range variants {
		cv := shopify.CatalogVariant{SKU: v.Sku, EAN: v.Ean}
		_ = json.Unmarshal(v.OptionValues, &cv.OptionValues)
		if v.PriceCents != nil {
			cv.PriceCents = *v.PriceCents
		}
		if v.CompareAtCents != nil {
			cv.CompareAtCents = *v.CompareAtCents
		}
		if v.SupplierActive != nil && *v.SupplierActive && v.Availability != nil {
			var av availability.Availability
			if err := json.Unmarshal(v.Availability, &av); err == nil {
				cv.Availability = &av
			}
		}
		byProduct[v.ProductID] = append(byProduct[v.ProductID], cv)
	}

	plan := SyncPlan{
		StoreConfigured: os.Getenv("SHOPIFY_STORE_DOMAIN") != "" && os.Getenv("SHOPIFY_CLIENT_ID") != "",
		WriteEnabled:    os.Getenv("SHOPIFY_WRITE_ENABLED") == "true",
		Products:        make([]PlanProduct, 0, len(rows)),
	}
	now := time.Now()
	for _, row := range rows {
		cp := shopify.CatalogProduct{Title: row.Name, DescriptionHTML: row.DescriptionHtml, Vendor: row.Brand,
			ProductType: row.Category, Active: row.Status == "active", Variants: byProduct[row.ID]}
		_ = json.Unmarshal(row.OptionNames, &cp.Options)
		_ = json.Unmarshal(row.ImageUrls, &cp.ImageURLs)
		next := shopify.Project(cp, now)

		var synced *shopify.Product
		if row.SyncedProjection != nil {
			var p shopify.Product
			if err := json.Unmarshal(row.SyncedProjection, &p); err == nil {
				synced = &p
			}
		}
		item := PlanProduct{ProductID: row.ID, Name: row.Brand + " " + row.Name, InShopify: row.ShopifyProductID != nil,
			LastSynced: row.LastSyncedAt, LastError: row.LastError, Ops: shopify.Diff(synced, next), Issues: []string{},
			Projection: next}
		if item.Ops == nil {
			item.Ops = []shopify.Op{}
		}
		for _, e := range shopify.Validate(next) {
			item.Issues = append(item.Issues, e.Error())
		}
		plan.Products = append(plan.Products, item)
	}
	writeJSON(w, http.StatusOK, plan)
}
