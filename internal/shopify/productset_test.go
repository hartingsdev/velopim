package shopify

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProductSetInput(t *testing.T) {
	p := Product{
		Title: "City 3 rot", DescriptionHTML: "<p>Text</p>", Vendor: "Beispielrad", ProductType: "Citybike",
		Options: []string{"Farbe", "Rahmenhöhe"}, ImageURLs: []string{"https://example.test/a.jpg"},
		Variants: []Variant{
			{SKU: "10001", Barcode: "4006381333931", OptionValues: []string{"rot", "50"}, PriceCents: 89900, CompareAtCents: 99905, Sellable: true, DeliveryText: "Sofort lieferbar"},
			{SKU: "10002", OptionValues: []string{"rot", "54"}, PriceCents: 89900, Sellable: false, DeliveryText: "Derzeit nicht lieferbar"},
			{SKU: "10003", OptionValues: []string{"blau", ""}, PriceCents: 5, Sellable: true, DeliveryText: "Sofort lieferbar"},
		},
	}
	raw, err := json.Marshal(ProductSetInput(p, "gid://shopify/Location/1"))
	if err != nil {
		t.Fatal(err)
	}
	var in struct {
		Title, DescriptionHTML, Vendor, ProductType, Status string
		ProductOptions                                      []struct {
			Name     string
			Position int
			Values   []struct{ Name string }
		}
		Variants []struct {
			SKU, Price, InventoryPolicy string
			CompareAtPrice              *string
			Barcodes                    []struct{ Value string }
			OptionValues                []struct{ OptionName, Name string }
			InventoryItem               struct{ Tracked bool }
			InventoryQuantities         []struct {
				LocationID, Name string
				Quantity         *int
			}
			Metafields []struct{ Namespace, Key, Type, Value string }
		}
		Files []struct{ OriginalSource, ContentType string }
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatal(err)
	}
	if in.Title != "City 3 rot" || in.DescriptionHTML != "<p>Text</p>" || in.Vendor != "Beispielrad" || in.ProductType != "Citybike" {
		t.Errorf("product fields: %+v", in)
	}
	if in.Status != "DRAFT" {
		t.Errorf("status = %s, an unpublished product must be a draft", in.Status)
	}
	// Options list their values in order of first appearance.
	if len(in.ProductOptions) != 2 || in.ProductOptions[0].Name != "Farbe" || len(in.ProductOptions[0].Values) != 2 ||
		in.ProductOptions[0].Values[0].Name != "rot" || in.ProductOptions[1].Position != 2 || len(in.ProductOptions[1].Values) != 3 {
		t.Errorf("options: %+v", in.ProductOptions)
	}
	v := in.Variants
	if v[0].Price != "899.00" || v[0].CompareAtPrice == nil || *v[0].CompareAtPrice != "999.05" || v[2].Price != "0.05" {
		t.Errorf("money: %s %v %s", v[0].Price, v[0].CompareAtPrice, v[2].Price)
	}
	if v[1].CompareAtPrice != nil {
		t.Error("a variant without compare-at price must clear it")
	}
	if len(v[0].Barcodes) != 1 || v[0].Barcodes[0].Value != "4006381333931" || len(v[1].Barcodes) != 0 {
		t.Errorf("barcodes: %+v %+v", v[0].Barcodes, v[1].Barcodes)
	}
	// Sellable: not tracked. Not sellable: tracked with zero stock, no overselling.
	if v[0].InventoryItem.Tracked || len(v[0].InventoryQuantities) != 0 {
		t.Errorf("sellable variant must not be tracked: %+v", v[0])
	}
	if !v[1].InventoryItem.Tracked || len(v[1].InventoryQuantities) != 1 || *v[1].InventoryQuantities[0].Quantity != 0 ||
		v[1].InventoryQuantities[0].LocationID != "gid://shopify/Location/1" || v[1].InventoryPolicy != "DENY" {
		t.Errorf("unsellable variant must be tracked at zero: %+v", v[1])
	}
	if m := v[1].Metafields; len(m) != 1 || m[0].Namespace != "velopim" || m[0].Key != "delivery_time" || m[0].Value != "Derzeit nicht lieferbar" {
		t.Errorf("delivery metafield: %+v", m)
	}
	// A missing option value still needs a name.
	if v[2].OptionValues[1].Name != "–" || v[2].OptionValues[0].OptionName != "Farbe" {
		t.Errorf("option values: %+v", v[2].OptionValues)
	}
	if len(in.Files) != 1 || in.Files[0].OriginalSource != "https://example.test/a.jpg" || in.Files[0].ContentType != "IMAGE" {
		t.Errorf("files: %+v", in.Files)
	}

	// Only the fields of the projection are sent: no purchase price, no notes.
	for _, forbidden := range []string{"cost", "note", "supplier"} {
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Errorf("input contains %q: %s", forbidden, raw)
		}
	}

	// A product without options gets Shopify's default option; a published
	// product is active.
	plain := ProductSetInput(Product{Title: "Solo", Published: true, Variants: []Variant{{SKU: "1", PriceCents: 100, Sellable: true}}}, "loc")
	raw, _ = json.Marshal(plain)
	if !strings.Contains(string(raw), `"status":"ACTIVE"`) || !strings.Contains(string(raw), `"name":"Title"`) ||
		!strings.Contains(string(raw), `"name":"Default Title","optionName":"Title"`) {
		t.Errorf("plain product: %s", raw)
	}
}
