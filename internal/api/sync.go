package api

import (
	"errors"
	"net/http"
	"os"
	"time"

	"velopim/internal/db"
	"velopim/internal/shopify"
	"velopim/internal/shopsync"
)

// SyncPlan is the response of GET /api/sync-plan: what a sync would send to
// Shopify. Reading the plan sends nothing.
type SyncPlan struct {
	// StoreConfigured reports whether Shopify credentials are set.
	StoreConfigured bool `json:"store_configured"`
	// WriteEnabled mirrors SHOPIFY_WRITE_ENABLED.
	WriteEnabled bool          `json:"write_enabled"`
	Products     []PlanProduct `json:"products"`
}

// PlanProduct is the planned sync of one product.
type PlanProduct struct {
	ProductID int64  `json:"product_id"`
	Name      string `json:"name"`
	// Wanted is false for a product taken out of the store that still
	// exists in Shopify; the sync sets it back to draft.
	Wanted     bool            `json:"wanted"`
	InShopify  bool            `json:"in_shopify"`
	LastSynced *time.Time      `json:"last_synced_at"`
	LastError  *string         `json:"last_error"`
	Ops        []shopify.Op    `json:"ops"`
	Issues     []string        `json:"issues"` // why Shopify would refuse the product
	Projection shopify.Product `json:"projection"`
}

func (s *server) syncPlan(w http.ResponseWriter, r *http.Request) {
	items, err := shopsync.Plan(r.Context(), db.New(s.pool), time.Now())
	if err != nil {
		internalError(w, err)
		return
	}
	plan := SyncPlan{
		StoreConfigured: s.store != nil,
		WriteEnabled:    os.Getenv("SHOPIFY_WRITE_ENABLED") == "true",
		Products:        make([]PlanProduct, 0, len(items)),
	}
	for _, item := range items {
		plan.Products = append(plan.Products, PlanProduct{ProductID: item.ProductID, Name: item.Name, Wanted: item.Wanted,
			InShopify: item.ShopifyID != "", LastSynced: item.LastSynced, LastError: item.LastError, Ops: item.Ops,
			Issues: item.Issues, Projection: item.Next})
	}
	writeJSON(w, http.StatusOK, plan)
}

// runSync sends the pending changes to Shopify. It answers 409 when no
// store is configured or writing is switched off.
func (s *server) runSync(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if !readJSON(w, r, &in) {
		return
	}
	if s.store == nil {
		writeError(w, http.StatusConflict, "no Shopify store is configured")
		return
	}
	res, err := shopsync.Run(r.Context(), s.pool, s.store, time.Now())
	switch {
	case errors.Is(err, shopify.ErrWritesDisabled):
		writeError(w, http.StatusConflict, "writing to Shopify is switched off (SHOPIFY_WRITE_ENABLED)")
	case errors.Is(err, shopify.ErrNotDevStore):
		writeError(w, http.StatusConflict, "the configured store is not a development store")
	case err != nil:
		internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, res)
	}
}
