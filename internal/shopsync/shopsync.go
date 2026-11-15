// Package shopsync brings the products selected for the store to Shopify:
// it computes the projection of each product, compares it with what Shopify
// last confirmed and sends the difference.
package shopsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/availability"
	"velopim/internal/db"
	"velopim/internal/shopify"
)

// Store is the part of the Shopify client the sync needs.
type Store interface {
	SetProduct(ctx context.Context, p shopify.Product, shopifyID string, mediaIDs map[string]string) (shopify.SetResult, error)
	// WaitForMedia reports the state of the product's images once Shopify
	// has fetched them, or after the limit.
	WaitForMedia(ctx context.Context, shopifyID string, limit time.Duration) ([]shopify.MediaState, error)
	AdminURL(numericID string) string
}

// mediaWait is how long a run waits for Shopify to fetch a product's images.
const mediaWait = 30 * time.Second

// Item is the planned sync of one product.
type Item struct {
	ProductID int64
	Name      string
	// Wanted is false for a product that was taken out of the store but
	// still exists in Shopify; it is set back to draft there.
	Wanted     bool
	ShopifyID  string // empty until the product exists in Shopify
	LastSynced *time.Time
	LastError  *string
	// Next is what Shopify should show; Ops is the difference to what it
	// last confirmed. No ops: nothing to send.
	Next shopify.Product
	Ops  []shopify.Op
	// Issues are reasons Shopify would refuse the product; it is not sent.
	Issues []string
	// MediaIDs maps image addresses to the media Shopify already holds.
	MediaIDs map[string]string
}

// Plan computes what a sync would do. It reads only.
func Plan(ctx context.Context, q *db.Queries, now time.Time) ([]Item, error) {
	rows, err := q.ListWantedProducts(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	variants, err := q.ListVariantsForProjection(ctx, ids)
	if err != nil {
		return nil, err
	}
	byProduct := map[int64][]shopify.CatalogVariant{}
	for _, v := range variants {
		cv := shopify.CatalogVariant{SKU: v.Sku, EAN: v.Ean}
		if err := json.Unmarshal(v.OptionValues, &cv.OptionValues); err != nil {
			return nil, fmt.Errorf("shopsync: option values of %s: %w", v.Sku, err)
		}
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

	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		cp := shopify.CatalogProduct{Title: row.Name, DescriptionHTML: row.DescriptionHtml, Vendor: row.Brand,
			ProductType: row.Category, Active: row.Status == "active" && row.Wanted, Variants: byProduct[row.ID]}
		if err := errors.Join(json.Unmarshal(row.OptionNames, &cp.Options), json.Unmarshal(row.ImageUrls, &cp.ImageURLs)); err != nil {
			return nil, fmt.Errorf("shopsync: product %d: %w", row.ID, err)
		}
		item := Item{ProductID: row.ID, Name: row.Brand + " " + row.Name, Wanted: row.Wanted, LastSynced: row.LastSyncedAt,
			LastError: row.LastError, Next: shopify.Project(cp, now), Issues: []string{}, MediaIDs: map[string]string{}}
		_ = json.Unmarshal(row.MediaIds, &item.MediaIDs)
		if row.ShopifyProductID != nil {
			item.ShopifyID = *row.ShopifyProductID
		}
		var synced *shopify.Product
		if row.SyncedProjection != nil {
			var p shopify.Product
			if err := json.Unmarshal(row.SyncedProjection, &p); err == nil {
				synced = &p
			}
		}
		item.Ops = shopify.Diff(synced, item.Next)
		if item.Ops == nil {
			item.Ops = []shopify.Op{}
		}
		for _, e := range shopify.Validate(item.Next) {
			item.Issues = append(item.Issues, e.Error())
		}
		items = append(items, item)
	}
	return items, nil
}

// Outcome is what happened to one product in a run.
type Outcome struct {
	ProductID int64  `json:"product_id"`
	Name      string `json:"name"`
	// Status is "synced", "unchanged" or "failed".
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	// Created is true when the product was new to Shopify.
	Created bool `json:"created,omitempty"`
}

// Result summarizes a run.
type Result struct {
	Synced    int       `json:"synced"`
	Unchanged int       `json:"unchanged"`
	Failed    int       `json:"failed"`
	Products  []Outcome `json:"products"`
}

// Run sends every pending difference to Shopify. Each product is handled on
// its own: one failing product does not stop the others. A failure is stored
// on the listing and becomes a task; a new product becomes a task to check
// and publish it (docs/decisions/0025). Running again without changes sends
// nothing.
func Run(ctx context.Context, pool *pgxpool.Pool, store Store, now time.Time) (Result, error) {
	items, err := Plan(ctx, db.New(pool), now)
	if err != nil {
		return Result{}, err
	}
	res := Result{Products: []Outcome{}}
	for _, item := range items {
		out := Outcome{ProductID: item.ProductID, Name: item.Name}
		switch {
		case len(item.Ops) == 0:
			out.Status = "unchanged"
			res.Unchanged++
		case len(item.Issues) > 0:
			err = fail(ctx, pool, item, "Shopify würde das Produkt ablehnen: "+joinIssues(item.Issues))
			out.Status, out.Error = "failed", joinIssues(item.Issues)
			res.Failed++
		default:
			var set shopify.SetResult
			set, err = store.SetProduct(ctx, item.Next, item.ShopifyID, item.MediaIDs)
			if err != nil {
				// Without permission to write nothing is recorded: that is
				// a configuration problem, not one of the product.
				if errors.Is(err, shopify.ErrWritesDisabled) || errors.Is(err, shopify.ErrNotDevStore) {
					return res, err
				}
				out.Status, out.Error = "failed", err.Error()
				res.Failed++
				err = fail(ctx, pool, item, err.Error())
				break
			}
			out.Status, out.Created = "synced", item.ShopifyID == ""
			res.Synced++
			mediaIDs, imageProblem := checkImages(ctx, store, item, set)
			out.Error = imageProblem
			err = succeed(ctx, pool, item, set, store.AdminURL(set.NumericID), mediaIDs, imageProblem, now)
		}
		if err != nil {
			return res, err
		}
		res.Products = append(res.Products, out)
	}
	return res, nil
}

func joinIssues(issues []string) string {
	s := ""
	for i, issue := range issues {
		if i > 0 {
			s += "; "
		}
		s += issue
	}
	return s
}

// checkImages waits for Shopify to fetch the product's images. It returns
// the media ids of the images that are ready, by address, and a message
// naming the images Shopify could not load.
func checkImages(ctx context.Context, store Store, item Item, set shopify.SetResult) (map[string]string, string) {
	ready := map[string]string{}
	urls := item.Next.ImageURLs
	if len(urls) == 0 || len(set.MediaIDs) != len(urls) {
		return ready, ""
	}
	states, err := store.WaitForMedia(ctx, set.ProductID, mediaWait)
	if err != nil {
		return ready, "Zustand der Bilder konnte nicht gelesen werden: " + err.Error()
	}
	status := map[string]string{}
	for _, s := range states {
		status[s.ID] = s.Status
	}
	var failed []string
	for i, u := range urls {
		switch status[set.MediaIDs[i]] {
		case "READY":
			ready[u] = set.MediaIDs[i]
		case "FAILED":
			failed = append(failed, u)
		}
	}
	if len(failed) > 0 {
		return ready, "Shopify konnte diese Bilder nicht laden: " + joinIssues(failed)
	}
	return ready, ""
}

// succeed stores what Shopify confirmed. A product new to Shopify gets the
// task to check and publish it. imageProblem names images Shopify could not
// load; it is kept as the listing's error and becomes a task.
func succeed(ctx context.Context, pool *pgxpool.Pool, item Item, set shopify.SetResult, adminURL string, mediaIDs map[string]string, imageProblem string, now time.Time) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
	q := db.New(tx)

	projection, _ := json.Marshal(item.Next)
	variantIDs, _ := json.Marshal(set.VariantIDs)
	hash := item.Next.Hash()
	media, _ := json.Marshal(mediaIDs)
	var lastError *string
	if imageProblem != "" {
		lastError = &imageProblem
	}
	if err := q.SaveListingSynced(ctx, db.SaveListingSyncedParams{ProductID: item.ProductID, ShopifyProductID: &set.ProductID,
		VariantIds: variantIDs, SyncedProjection: projection, SyncedHash: &hash, LastSyncedAt: &now,
		LastError: lastError, MediaIds: media}); err != nil {
		return err
	}
	if err := q.CloseSyncFailedTasks(ctx, &item.ProductID); err != nil {
		return err
	}
	if imageProblem != "" {
		if err := q.InsertProductTask(ctx, db.InsertProductTaskParams{Kind: "sync_failed",
			Title: "Bilder fehlen in Shopify: " + item.Name, Detail: imageProblem, Priority: 2, ProductID: &item.ProductID}); err != nil {
			return err
		}
	}
	if item.ShopifyID == "" && !item.Next.Published {
		open, err := q.HasOpenProductTask(ctx, db.HasOpenProductTaskParams{ProductID: item.ProductID, Kind: "publish_product"})
		if err != nil {
			return err
		}
		if !open {
			detail := "Das Produkt liegt als Entwurf in Shopify. Prüfe es dort (Vorschau) und gib es dann hier frei: " + adminURL +
				"\nFreigeben setzt das Produkt aktiv; die nächste Übertragung veröffentlicht es. Ablehnen lässt es als Entwurf stehen."
			if err := q.InsertProductTask(ctx, db.InsertProductTaskParams{Kind: "publish_product",
				Title: "Veröffentlichen: " + item.Name, Detail: detail, Priority: 2, ProductID: &item.ProductID}); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

// fail stores the error on the listing and makes sure there is one open
// task about it.
func fail(ctx context.Context, pool *pgxpool.Pool, item Item, message string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
	q := db.New(tx)
	if err := q.SaveListingError(ctx, db.SaveListingErrorParams{ProductID: item.ProductID, LastError: &message}); err != nil {
		return err
	}
	open, err := q.HasOpenProductTask(ctx, db.HasOpenProductTaskParams{ProductID: item.ProductID, Kind: "sync_failed"})
	if err != nil {
		return err
	}
	if !open {
		if err := q.InsertProductTask(ctx, db.InsertProductTaskParams{Kind: "sync_failed",
			Title: "Übertragung zu Shopify fehlgeschlagen: " + item.Name, Detail: message, Priority: 1, ProductID: &item.ProductID}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
