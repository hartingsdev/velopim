package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"velopim/internal/connector"
	"velopim/internal/db"
)

// Resplit cuts the products of a model anew by other split axes
// (docs/decisions/0021). Variants keep their article numbers, prices and
// history; they are moved to newly created products and the old products
// are removed. Notes, tasks and the product history move to the first new
// product. The new products are drafts and not selected for the store.
//
// It is refused with a BlockedError once a product of the model exists in
// Shopify: there the cut is part of the public addresses.
func Resplit(ctx context.Context, q *db.Queries, productID int64, splitBy []Axis, actor string) error {
	model, err := q.GetProductModel(ctx, productID)
	if err != nil {
		return err
	}
	if model.ModelKey == "" {
		return &BlockedError{Issues: []string{"Das Produkt gehört zu keinem Modell und lässt sich nicht neu trennen"}}
	}
	for _, axis := range splitBy {
		if !axis.Valid() {
			return &BlockedError{Issues: []string{fmt.Sprintf("unbekanntes Trennmerkmal %q", axis)}}
		}
	}
	old, err := q.ListModelProducts(ctx, model.ModelKey)
	if err != nil {
		return err
	}
	for _, p := range old {
		if p.InShopify {
			return &BlockedError{Issues: []string{"Das Modell ist schon in Shopify. Die Trennung lässt sich dort nicht mehr ändern, ohne die Produkte neu anzulegen"}}
		}
	}
	rows, err := q.ListModelVariants(ctx, model.ModelKey)
	if err != nil {
		return err
	}
	total, err := q.CountModelVariants(ctx, model.ModelKey)
	if err != nil {
		return err
	}
	if int64(len(rows)) != total {
		return &BlockedError{Issues: []string{"Nicht jede Variante hat einen Lieferantenartikel; ohne dessen Merkmale lässt sich nicht neu trennen"}}
	}
	if len(rows) == 0 || len(old) == 0 {
		return nil
	}

	// All articles are treated as one model, whatever their names are now.
	first := rows[0]
	items := make([]connector.SupplierItem, len(rows))
	byKey := map[string]db.ListModelVariantsRow{}
	for i, row := range rows {
		key := row.SourceID + "\x1f" + row.Sku
		it := connector.SupplierItem{SKU: key, Brand: first.Brand, Name: first.Name, Color: row.Color, FrameSize: row.FrameSize}
		if first.ModelYear != nil {
			it.ModelYear = int(*first.ModelYear)
		}
		if err := json.Unmarshal(row.Attributes, &it.Attributes); err != nil {
			return fmt.Errorf("catalog: attributes of %s: %w", row.Sku, err)
		}
		items[i] = it
		byKey[key] = row
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

	policies, err := LoadUsagePolicies(ctx, q)
	if err != nil {
		return err
	}
	useImages := policies.For(first.SourceID, first.Brand, GroupImages) == PolicyUse

	oldIDs := make([]int64, len(old))
	oldTextIDs := make([]string, len(old))
	for i, p := range old {
		oldIDs[i] = p.ID
		oldTextIDs[i] = strconv.FormatInt(p.ID, 10)
	}
	oldSplit := string(compactJSON(old[0].SplitBy))
	newSplit := string(mustJSON(nonNilAxes(splitBy)))
	reason := "Trennung des Modells geändert"

	var firstNew int64
	for _, p := range products {
		var images []string
		seen := map[string]bool{}
		for _, v := range p.Variants {
			if !useImages {
				break
			}
			var urls []string
			_ = json.Unmarshal(byKey[v.SKU].ImageUrls, &urls)
			for _, u := range urls {
				if !seen[u] {
					seen[u] = true
					images = append(images, u)
				}
			}
		}
		// Texts may have been edited by hand: they come from the old product.
		id, err := q.InsertProduct(ctx, db.InsertProductParams{
			Brand: p.Model.Brand, Name: p.Title, ModelYear: first.ModelYear, Category: old[0].Category,
			DescriptionHtml: old[0].DescriptionHtml, ImageUrls: mustJSON(nonNil(images)),
			OptionNames: mustJSON(nonNil(p.OptionNames)), ModelKey: model.ModelKey,
			SplitBy: []byte(newSplit), SplitValues: mustJSON(nonNilSplit(p.Split))})
		if err != nil {
			return err
		}
		if firstNew == 0 {
			firstNew = id
		}
		for _, v := range p.Variants {
			if err := q.MoveVariant(ctx, db.MoveVariantParams{ID: byKey[v.SKU].VariantID, ProductID: id,
				OptionValues: mustJSON(nonNil(v.OptionValues))}); err != nil {
				return err
			}
		}
		if err := q.InsertCatalogFieldChange(ctx, db.InsertCatalogFieldChangeParams{EntityType: "product", EntityID: id,
			Field: "split_by", OldValue: &oldSplit, NewValue: &newSplit, Origin: "user", Actor: &actor, Reason: &reason}); err != nil {
			return err
		}
	}

	if err := q.MoveProductReferences(ctx, db.MoveProductReferencesParams{Target: firstNew, OldTextIds: oldTextIDs, OldIds: oldIDs}); err != nil {
		return err
	}
	if err := q.DeleteListings(ctx, oldIDs); err != nil {
		return err
	}
	return q.DeleteProducts(ctx, oldIDs)
}

// SplitAxes returns the axes in which the variants of a product's model
// differ, i.e. what the model can sensibly be split by.
func SplitAxes(ctx context.Context, q *db.Queries, productID int64) ([]Axis, error) {
	model, err := q.GetProductModel(ctx, productID)
	if err != nil || model.ModelKey == "" {
		return nil, err
	}
	rows, err := q.ListModelVariants(ctx, model.ModelKey)
	if err != nil {
		return nil, err
	}
	values := map[Axis]map[string]bool{}
	var order []Axis
	add := func(axis Axis, value string) {
		if values[axis] == nil {
			values[axis] = map[string]bool{}
			order = append(order, axis)
		}
		values[axis][value] = true
	}
	for _, row := range rows {
		add(AxisColor, row.Color)
		add(AxisFrameSize, row.FrameSize)
	}
	keys := map[string]bool{}
	for _, row := range rows {
		var attrs map[string]string
		_ = json.Unmarshal(row.Attributes, &attrs)
		for k := range attrs {
			keys[k] = true
		}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		add(Axis(attributePrefix+k), "")
	}
	// Attribute values per variant, counting a missing attribute as empty.
	for _, axis := range order {
		if axis == AxisColor || axis == AxisFrameSize {
			continue
		}
		values[axis] = map[string]bool{}
		for _, row := range rows {
			var attrs map[string]string
			_ = json.Unmarshal(row.Attributes, &attrs)
			values[axis][attrs[string(axis)[len(attributePrefix):]]] = true
		}
	}
	var out []Axis
	for _, axis := range order {
		if len(values[axis]) > 1 {
			out = append(out, axis)
		}
	}
	return out, nil
}
