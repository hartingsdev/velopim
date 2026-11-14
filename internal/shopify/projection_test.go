package shopify

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func sample() Product {
	return Product{
		Title: "Trekking 5.4 (2027)", DescriptionHTML: "<p>Text</p>", Vendor: "Beispielrad", ProductType: "Trekkingrad",
		Published: true,
		Options:   []string{"Farbe", "Rahmenhöhe"},
		Variants: []Variant{
			{SKU: "100-001", Barcode: "4006381333931", OptionValues: []string{"schwarz", "52"}, PriceCents: 169900, CompareAtCents: 179900, Sellable: true, DeliveryText: "Lieferzeit 3–5 Werktage"},
			{SKU: "100-002", Barcode: "4006381333948", OptionValues: []string{"schwarz", "56"}, PriceCents: 169900, CompareAtCents: 179900, Sellable: false, DeliveryText: "Derzeit nicht lieferbar"},
		},
		ImageURLs:  []string{"a.jpg", "b.jpg"},
		Metafields: map[string]string{"custom.model_year": "2027"},
	}
}

func TestDiffNeverSyncedCreates(t *testing.T) {
	got := Diff(nil, sample())
	if !reflect.DeepEqual(got, []Op{{Kind: CreateProduct}}) {
		t.Errorf("got %+v", got)
	}
}

func TestDiffIsIdempotent(t *testing.T) {
	p := sample()
	if got := Diff(&p, sample()); len(got) != 0 {
		t.Errorf("identical projections produced writes: %+v", got)
	}
	if p.Hash() != sample().Hash() {
		t.Error("identical projections have different hashes")
	}
}

func TestDiff(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Product)
		want   []Op
	}{
		{"title and draft", func(p *Product) { p.Title = "Neu"; p.Published = false },
			[]Op{{Kind: UpdateProduct, Fields: []string{"title", "published"}}}},
		{"image order", func(p *Product) { p.ImageURLs = []string{"b.jpg", "a.jpg"} },
			[]Op{{Kind: SetImages}}},
		{"metafield changed, added and removed", func(p *Product) {
			p.Metafields = map[string]string{"custom.model_year": "2028", "custom.frame": "Diamant"}
		}, []Op{{Kind: SetMetafields, Fields: []string{"custom.frame", "custom.model_year"}}}},
		{"metafield removed", func(p *Product) { p.Metafields = nil },
			[]Op{{Kind: SetMetafields, Fields: []string{"custom.model_year"}}}},
		{"price and availability of one variant", func(p *Product) {
			p.Variants[1].PriceCents = 159900
			p.Variants[1].Sellable = true
			p.Variants[1].DeliveryText = "Sofort lieferbar"
		}, []Op{{Kind: UpdateVariant, SKU: "100-002", Fields: []string{"price", "sellable", "delivery_text"}}}},
		{"variant added and removed", func(p *Product) {
			p.Variants = []Variant{p.Variants[0], {SKU: "100-003", OptionValues: []string{"schwarz", "60"}, PriceCents: 169900}}
		}, []Op{{Kind: CreateVariant, SKU: "100-003"}, {Kind: DeleteVariant, SKU: "100-002"}}},
		{"variant order alone is no change", func(p *Product) {
			p.Variants[0], p.Variants[1] = p.Variants[1], p.Variants[0]
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synced, next := sample(), sample()
			tt.change(&next)
			got := Diff(&synced, next)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
			if tt.want != nil && synced.Hash() == next.Hash() {
				t.Error("changed projection has the same hash")
			}
		})
	}
}

func TestValidate(t *testing.T) {
	if errs := Validate(sample()); len(errs) != 0 {
		t.Fatalf("valid product rejected: %v", errs)
	}
	tests := []struct {
		name   string
		change func(*Product)
		want   string
	}{
		{"empty title", func(p *Product) { p.Title = " " }, "title is empty"},
		{"too many options", func(p *Product) { p.Options = []string{"a", "b", "c", "d"} }, "Shopify allows 3"},
		{"no variants", func(p *Product) { p.Variants = nil }, "no variants"},
		{"duplicate SKU", func(p *Product) { p.Variants[1].SKU = "100-001" }, "used twice"},
		{"missing option value", func(p *Product) { p.Variants[0].OptionValues = []string{"schwarz"} }, "option values for 2 options"},
		{"same option values", func(p *Product) { p.Variants[1].OptionValues = []string{"schwarz", "52"} }, "same option values"},
		{"no price", func(p *Product) { p.Variants[0].PriceCents = 0; p.Variants[0].CompareAtCents = 0 }, "has no price"},
		{"compare-at below price", func(p *Product) { p.Variants[0].CompareAtCents = 100 }, "compare-at price"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := sample()
			tt.change(&p)
			errs := Validate(p)
			found := false
			for _, e := range errs {
				found = found || strings.Contains(e.Error(), tt.want)
			}
			if !found {
				t.Errorf("want an error containing %q, got %v", tt.want, errs)
			}
		})
	}
}

// The projection is a positive list. This test fails when someone adds a
// field, so that adding one is always a deliberate decision about what
// becomes public.
func TestProjectionFieldsArePinned(t *testing.T) {
	b, _ := json.Marshal(Product{Variants: []Variant{{}}, Metafields: map[string]string{"k": "v"}})
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	keys := func(m map[string]any) []string {
		var out []string
		for k := range m {
			out = append(out, k)
		}
		return out
	}
	product := keys(m)
	variant := keys(m["variants"].([]any)[0].(map[string]any))
	wantProduct := []string{"title", "description_html", "vendor", "product_type", "published", "options", "variants", "image_urls", "metafields"}
	wantVariant := []string{"sku", "barcode", "option_values", "price_cents", "compare_at_cents", "sellable", "delivery_text"}
	if !sameSet(product, wantProduct) {
		t.Errorf("product fields changed: %v", product)
	}
	if !sameSet(variant, wantVariant) {
		t.Errorf("variant fields changed: %v", variant)
	}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]bool{}
	for _, x := range a {
		set[x] = true
	}
	for _, x := range b {
		if !set[x] {
			return false
		}
	}
	return true
}
