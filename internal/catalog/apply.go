package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"

	"velopim/internal/db"
)

// ApplyItemChange carries an accepted change of a supplier article over to
// the own catalog. It covers the fields that have a direct counterpart:
//
//	description  -> description of the product
//	image_urls   -> images of the product (union over its variants)
//	rrp_cents    -> compare-at price of the variant
//	ean          -> EAN of the variant
//
// The purchase price is handled by ShiftVariantPrice. Availability and
// deactivation need no copy: the Shopify projection reads them from the
// supplier article (docs/decisions/0027). Name, color, frame size and other
// attributes decide which product and variant an article is; changing them
// is not automated.
//
// Only the preferred supplier's article changes the catalog. Nothing happens
// when the article has no variant yet. origin is "user" or "rule".
func ApplyItemChange(ctx context.Context, q *db.Queries, actionID, supplierItemID int64, field, newValue, origin, actor string) error {
	switch field {
	case "description", "image_urls", "rrp_cents", "ean":
	default:
		return nil
	}
	v, err := q.GetVariantOfSupplierItem(ctx, supplierItemID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	record := func(entity string, id int64, field, oldValue, newValue string) error {
		if oldValue == newValue {
			return nil
		}
		reason := "Änderung des Lieferanten übernommen"
		params := db.InsertCatalogFieldChangeParams{EntityType: entity, EntityID: id, Field: field,
			OldValue: &oldValue, NewValue: &newValue, Origin: origin, ActionID: &actionID, Reason: &reason}
		if actor != "" {
			params.Actor = &actor
		}
		return q.InsertCatalogFieldChange(ctx, params)
	}

	switch field {
	case "description":
		if err := q.SetProductDescription(ctx, db.SetProductDescriptionParams{ID: v.ProductID, DescriptionHtml: newValue}); err != nil {
			return err
		}
		return record("product", v.ProductID, "description_html", v.DescriptionHtml, newValue)

	case "image_urls":
		old, err := q.GetProductImages(ctx, v.ProductID)
		if err != nil {
			return err
		}
		lists, err := q.ListSupplierImagesOfProduct(ctx, v.ProductID)
		if err != nil {
			return err
		}
		images := []string{}
		seen := map[string]bool{}
		for _, raw := range lists {
			var urls []string
			if err := json.Unmarshal(raw, &urls); err != nil {
				return err
			}
			for _, u := range urls {
				if !seen[u] {
					seen[u] = true
					images = append(images, u)
				}
			}
		}
		next := mustJSON(images)
		if err := q.SetProductImages(ctx, db.SetProductImagesParams{ID: v.ProductID, ImageUrls: next}); err != nil {
			return err
		}
		return record("product", v.ProductID, "image_urls", compactJSON(old), string(next))

	case "rrp_cents":
		rrp, err := strconv.ParseInt(newValue, 10, 64)
		if err != nil {
			return err
		}
		// The RRP is shown as compare-at price only while it is above the
		// sales price.
		var compareAt *int64
		if v.PriceCents != nil && rrp > *v.PriceCents {
			compareAt = &rrp
		}
		if err := q.SetVariantCompareAt(ctx, db.SetVariantCompareAtParams{ID: v.ID, CompareAtCents: compareAt}); err != nil {
			return err
		}
		return record("variant", v.ID, "compare_at_cents", centsText(v.CompareAtCents), centsText(compareAt))

	case "ean":
		if err := q.SetVariantEAN(ctx, db.SetVariantEANParams{ID: v.ID, Ean: newValue}); err != nil {
			return err
		}
		return record("variant", v.ID, "ean", v.Ean, newValue)
	}
	return nil
}

func centsText(c *int64) string {
	if c == nil {
		return ""
	}
	return strconv.FormatInt(*c, 10)
}

// compactJSON normalizes stored JSON so that it compares with marshalled JSON.
func compactJSON(raw []byte) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return string(mustJSON(v))
}
