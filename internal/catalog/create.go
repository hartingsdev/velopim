package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"velopim/internal/connector"
	"velopim/internal/db"
	"velopim/internal/pricing"
)

// BlockedError says why an approval cannot be carried out as asked, e.g.
// why the articles of a model cannot become products as cut.
type BlockedError struct{ Issues []string }

func (e *BlockedError) Error() string {
	return "catalog: not possible as asked: " + strings.Join(e.Issues, "; ")
}

// SKUPrefix starts every own article number (docs/decisions/0023).
const SKUPrefix = "VP-"

// CreateFromAction turns the articles of an approved new-model action into
// products and variants. Run it inside the transaction that approves the
// action.
//
// A model that already has products keeps its cut: splitBy is ignored and
// the new articles become variants of the matching products, or further
// products with the same split axes. Articles that left the feed or already
// have a variant are skipped. Variants get an own article number and, when a
// pricing rule matches, a sales price; without a rule the price stays empty.
func CreateFromAction(ctx context.Context, q *db.Queries, actionID int64, modelKey string, splitBy []Axis, actor string) error {
	rows, err := q.ListActionSupplierItems(ctx, actionID)
	if err != nil {
		return err
	}
	var items []connector.SupplierItem
	bySKU := map[string]db.ListActionSupplierItemsRow{}
	for _, row := range rows {
		if !row.Active {
			continue
		}
		linked, err := q.SupplierItemHasVariant(ctx, row.ID)
		if err != nil {
			return err
		}
		if linked {
			continue
		}
		it := connector.SupplierItem{SourceID: row.SourceID, SKU: row.Sku, EAN: row.Ean, Brand: row.Brand, Name: row.Name,
			Category: row.Category, Color: row.Color, FrameSize: row.FrameSize, Description: row.Description}
		if row.ModelYear != nil {
			it.ModelYear = int(*row.ModelYear)
		}
		if row.CostCents != nil {
			it.CostCents = *row.CostCents
		}
		if row.RrpCents != nil {
			it.RRPCents = *row.RrpCents
		}
		if err := json.Unmarshal(row.ImageUrls, &it.ImageURLs); err != nil {
			return fmt.Errorf("catalog: images of %s: %w", row.Sku, err)
		}
		if err := json.Unmarshal(row.Attributes, &it.Attributes); err != nil {
			return fmt.Errorf("catalog: attributes of %s: %w", row.Sku, err)
		}
		items = append(items, it)
		bySKU[row.Sku] = row
	}
	if len(items) == 0 {
		return nil
	}

	// An existing cut of the model wins over the request.
	existing, err := q.ListProductsOfModel(ctx, modelKey)
	if err != nil {
		return err
	}
	existingBySplit := map[string]int64{}
	for i, p := range existing {
		if i == 0 {
			splitBy = nil
			if err := json.Unmarshal(p.SplitBy, &splitBy); err != nil {
				return fmt.Errorf("catalog: split axes of product %d: %w", p.ID, err)
			}
		}
		var values []AxisValue
		if err := json.Unmarshal(p.SplitValues, &values); err != nil {
			return fmt.Errorf("catalog: split values of product %d: %w", p.ID, err)
		}
		existingBySplit[splitKey(values)] = p.ID
	}
	for _, axis := range splitBy {
		if !axis.Valid() {
			return &BlockedError{Issues: []string{fmt.Sprintf("unbekanntes Trennmerkmal %q", axis)}}
		}
	}

	products := Group(items, Settings{Default: Config{SplitBy: splitBy, Options: DefaultConfig.Options}})
	var blocked []string
	for _, p := range products {
		if p.Blocked {
			blocked = append(blocked, p.Issues...)
		}
	}
	if len(blocked) > 0 {
		return &BlockedError{Issues: blocked}
	}

	ruleRows, err := q.ListPricingRules(ctx)
	if err != nil {
		return err
	}
	priceRules := make([]pricing.Rule, 0, len(ruleRows))
	for _, row := range ruleRows {
		r := pricing.Rule{Brand: row.ScopeBrand, Category: row.ScopeCategory}
		if err := json.Unmarshal(row.Config, &r.Config); err != nil {
			return fmt.Errorf("catalog: pricing rule: %w", err)
		}
		priceRules = append(priceRules, r)
	}

	const reason = "Modell freigegeben"
	change := func(entity string, id int64, field, value string) error {
		return q.InsertCatalogFieldChange(ctx, db.InsertCatalogFieldChangeParams{
			EntityType: entity, EntityID: id, Field: field, NewValue: &value, Origin: "user",
			ActionID: &actionID, Actor: &actor, Reason: ptr(reason)})
	}

	for _, p := range products {
		first := bySKU[p.Variants[0].SKU]
		productID, known := existingBySplit[splitKey(p.Split)]
		if !known {
			var images []string
			seen := map[string]bool{}
			for _, v := range p.Variants {
				var urls []string
				_ = json.Unmarshal(bySKU[v.SKU].ImageUrls, &urls)
				for _, u := range urls {
					if !seen[u] {
						seen[u] = true
						images = append(images, u)
					}
				}
			}
			productID, err = q.InsertProduct(ctx, db.InsertProductParams{
				Brand: p.Model.Brand, Name: p.Title, ModelYear: first.ModelYear, Category: first.Category,
				DescriptionHtml: first.Description, ImageUrls: mustJSON(nonNil(images)),
				OptionNames: mustJSON(nonNil(p.OptionNames)), ModelKey: modelKey,
				SplitBy: mustJSON(nonNilAxes(splitBy)), SplitValues: mustJSON(nonNilSplit(p.Split))})
			if err != nil {
				return err
			}
			if err := change("product", productID, "status", "draft"); err != nil {
				return err
			}
		}

		for _, v := range p.Variants {
			row := bySKU[v.SKU]
			number, err := q.NextVariantNumber(ctx)
			if err != nil {
				return err
			}
			sku := SKUPrefix + strconv.FormatInt(number, 10)
			var price, compareAt *int64
			if cfg, ok := pricing.Pick(priceRules, row.Brand, row.Category); ok && row.CostCents != nil {
				var rrp int64
				if row.RrpCents != nil {
					rrp = *row.RrpCents
				}
				if res, err := pricing.Calculate(*row.CostCents, rrp, cfg); err == nil {
					price = &res.PriceCents
					if res.CompareAtCents > 0 {
						compareAt = &res.CompareAtCents
					}
				}
			}
			variantID, err := q.InsertVariant(ctx, db.InsertVariantParams{
				ProductID: productID, Sku: sku, Ean: row.Ean, OptionValues: mustJSON(nonNil(v.OptionValues)),
				PriceCents: price, CompareAtCents: compareAt})
			if err != nil {
				return err
			}
			if err := q.LinkVariantSupplierItem(ctx, db.LinkVariantSupplierItemParams{VariantID: variantID, SupplierItemID: row.ID}); err != nil {
				return err
			}
			if err := change("variant", variantID, "sku", sku); err != nil {
				return err
			}
			if price != nil {
				if err := change("variant", variantID, "price_cents", strconv.FormatInt(*price, 10)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ShiftVariantPrice changes a variant's sales price by delta cents and
// records the change. origin is "user" or "rule"; actor may be empty for
// rules. It fails with a BlockedError when the variant has no price or the
// result would not be positive.
func ShiftVariantPrice(ctx context.Context, q *db.Queries, variantID, delta int64, origin string, actionID int64, actor, reason string) error {
	if delta == 0 {
		return nil
	}
	price, err := q.ShiftVariantPrice(ctx, db.ShiftVariantPriceParams{ID: variantID, DeltaCents: delta})
	if errors.Is(err, pgx.ErrNoRows) {
		return &BlockedError{Issues: []string{"Die Variante hat keinen Verkaufspreis, der sich um diesen Betrag ändern lässt"}}
	}
	if err != nil {
		return err
	}
	oldValue, newValue := strconv.FormatInt(*price-delta, 10), strconv.FormatInt(*price, 10)
	params := db.InsertCatalogFieldChangeParams{
		EntityType: "variant", EntityID: variantID, Field: "price_cents", OldValue: &oldValue, NewValue: &newValue,
		Origin: origin, ActionID: &actionID, Reason: &reason}
	if actor != "" {
		params.Actor = &actor
	}
	return q.InsertCatalogFieldChange(ctx, params)
}

func ptr[T any](v T) *T { return &v }

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // only plain strings and structs are passed
	}
	return b
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilAxes(s []Axis) []Axis {
	if s == nil {
		return []Axis{}
	}
	return s
}

func nonNilSplit(s []AxisValue) []AxisValue {
	if s == nil {
		return []AxisValue{}
	}
	return s
}
