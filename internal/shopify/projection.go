// Package shopify holds everything that goes to the shop. Product is the
// only shape the sync reads from: a field that is not part of it cannot
// reach Shopify (architecture decision 6). This file is pure; the API client
// lives separately.
package shopify

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Limits of the Shopify product model. Verify against the API version in
// use when the client is built.
const (
	MaxOptions  = 3
	MaxVariants = 2048
)

// Product is the public projection of one PIM product.
type Product struct {
	Title           string `json:"title"`
	DescriptionHTML string `json:"description_html"`
	Vendor          string `json:"vendor"`
	ProductType     string `json:"product_type"`
	// Published is false for drafts.
	Published bool `json:"published"`
	// Options are the option names, e.g. "Farbe", "Rahmenhöhe".
	Options   []string  `json:"options"`
	Variants  []Variant `json:"variants"`
	ImageURLs []string  `json:"image_urls"`
	// Metafields are product metafields keyed "namespace.key".
	Metafields map[string]string `json:"metafields,omitempty"`
}

// Variant is the public projection of one PIM variant. SKU is the stable key.
type Variant struct {
	SKU            string   `json:"sku"`
	Barcode        string   `json:"barcode"`
	OptionValues   []string `json:"option_values"`
	PriceCents     int64    `json:"price_cents"`
	CompareAtCents int64    `json:"compare_at_cents"`
	// Sellable decides whether customers can buy the variant. In the MVP no
	// stock quantities are written (architecture decision 9).
	Sellable bool `json:"sellable"`
	// DeliveryText goes to the delivery-time metafield of the variant.
	DeliveryText string `json:"delivery_text"`
}

// Hash identifies the content of a projection. It is stored after a
// successful sync; an unchanged hash means there is nothing to do.
func (p Product) Hash() string {
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Validate checks a projection against Shopify's product model before
// anything is sent.
func Validate(p Product) []error {
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if strings.TrimSpace(p.Title) == "" {
		fail("title is empty")
	}
	if len(p.Options) > MaxOptions {
		fail("%d options, Shopify allows %d", len(p.Options), MaxOptions)
	}
	if len(p.Variants) == 0 {
		fail("no variants")
	}
	if len(p.Variants) > MaxVariants {
		fail("%d variants, Shopify allows %d", len(p.Variants), MaxVariants)
	}
	skus, combos := map[string]bool{}, map[string]string{}
	for _, v := range p.Variants {
		if v.SKU == "" {
			fail("variant without SKU")
			continue
		}
		if skus[v.SKU] {
			fail("SKU %s is used twice", v.SKU)
		}
		skus[v.SKU] = true
		if len(v.OptionValues) != len(p.Options) {
			fail("variant %s has %d option values for %d options", v.SKU, len(v.OptionValues), len(p.Options))
			continue
		}
		for _, ov := range v.OptionValues {
			if strings.TrimSpace(ov) == "" {
				fail("variant %s has an empty option value", v.SKU)
			}
		}
		combo := strings.Join(v.OptionValues, "\x00")
		if other, ok := combos[combo]; ok {
			fail("variants %s and %s have the same option values", other, v.SKU)
		}
		combos[combo] = v.SKU
		if v.PriceCents <= 0 {
			fail("variant %s has no price", v.SKU)
		}
		if v.CompareAtCents != 0 && v.CompareAtCents <= v.PriceCents {
			fail("variant %s: compare-at price is not above the price", v.SKU)
		}
	}
	return errs
}

// OpKind is one kind of write to Shopify.
type OpKind string

const (
	CreateProduct OpKind = "create_product" // whole product including variants, images, metafields
	UpdateProduct OpKind = "update_product" // product fields named in Fields
	SetImages     OpKind = "set_images"
	SetMetafields OpKind = "set_metafields" // product metafield keys named in Fields
	CreateVariant OpKind = "create_variant"
	UpdateVariant OpKind = "update_variant" // variant fields named in Fields
	DeleteVariant OpKind = "delete_variant"
)

// Op is one write needed to bring Shopify from the last synced projection to
// the current one.
type Op struct {
	Kind   OpKind   `json:"kind"`
	SKU    string   `json:"sku,omitempty"`
	Fields []string `json:"fields,omitempty"`
}

// Diff returns the writes needed to get from synced (nil when the product
// was never synced) to next. Diff(&p, p) is empty, which makes the sync
// idempotent: repeating it after a success does nothing.
func Diff(synced *Product, next Product) []Op {
	if synced == nil {
		return []Op{{Kind: CreateProduct}}
	}
	var ops []Op

	var fields []string
	if synced.Title != next.Title {
		fields = append(fields, "title")
	}
	if synced.DescriptionHTML != next.DescriptionHTML {
		fields = append(fields, "description_html")
	}
	if synced.Vendor != next.Vendor {
		fields = append(fields, "vendor")
	}
	if synced.ProductType != next.ProductType {
		fields = append(fields, "product_type")
	}
	if synced.Published != next.Published {
		fields = append(fields, "published")
	}
	if !equal(synced.Options, next.Options) {
		fields = append(fields, "options")
	}
	if len(fields) > 0 {
		ops = append(ops, Op{Kind: UpdateProduct, Fields: fields})
	}

	if !equal(synced.ImageURLs, next.ImageURLs) {
		ops = append(ops, Op{Kind: SetImages})
	}

	var keys []string
	for k, v := range next.Metafields {
		if old, ok := synced.Metafields[k]; !ok || old != v {
			keys = append(keys, k)
		}
	}
	for k := range synced.Metafields {
		if _, ok := next.Metafields[k]; !ok {
			keys = append(keys, k)
		}
	}
	if len(keys) > 0 {
		sort.Strings(keys)
		ops = append(ops, Op{Kind: SetMetafields, Fields: keys})
	}

	old := make(map[string]Variant, len(synced.Variants))
	for _, v := range synced.Variants {
		old[v.SKU] = v
	}
	seen := make(map[string]bool, len(next.Variants))
	for _, v := range next.Variants {
		seen[v.SKU] = true
		o, ok := old[v.SKU]
		if !ok {
			ops = append(ops, Op{Kind: CreateVariant, SKU: v.SKU})
			continue
		}
		var vf []string
		if o.Barcode != v.Barcode {
			vf = append(vf, "barcode")
		}
		if !equal(o.OptionValues, v.OptionValues) {
			vf = append(vf, "option_values")
		}
		if o.PriceCents != v.PriceCents {
			vf = append(vf, "price")
		}
		if o.CompareAtCents != v.CompareAtCents {
			vf = append(vf, "compare_at_price")
		}
		if o.Sellable != v.Sellable {
			vf = append(vf, "sellable")
		}
		if o.DeliveryText != v.DeliveryText {
			vf = append(vf, "delivery_text")
		}
		if len(vf) > 0 {
			ops = append(ops, Op{Kind: UpdateVariant, SKU: v.SKU, Fields: vf})
		}
	}
	var removed []string
	for sku := range old {
		if !seen[sku] {
			removed = append(removed, sku)
		}
	}
	sort.Strings(removed)
	for _, sku := range removed {
		ops = append(ops, Op{Kind: DeleteVariant, SKU: sku})
	}
	return ops
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
