package shopify

import (
	"testing"
	"time"

	"velopim/internal/availability"
)

func TestProject(t *testing.T) {
	now := time.Date(2027, 1, 10, 0, 0, 0, 0, time.UTC)
	march := time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC)
	av := func(s availability.Status) *availability.Availability { return &availability.Availability{Status: s} }

	p := Project(CatalogProduct{
		Title: "City 3 rot", DescriptionHTML: "<p>Text</p>", Vendor: "Beispielrad", ProductType: "Citybike",
		Options: []string{"Rahmenhöhe"}, ImageURLs: []string{"https://example.test/a.jpg"},
		Variants: []CatalogVariant{
			{SKU: "10001", EAN: "4006381333931", OptionValues: []string{"46"}, PriceCents: 89900, CompareAtCents: 99900, Availability: av(availability.InStock)},
			{SKU: "10002", OptionValues: []string{"50"}, PriceCents: 89900, Availability: av(availability.Unavailable)},
			{SKU: "10003", OptionValues: []string{"54"}, PriceCents: 89900, Availability: &availability.Availability{Status: availability.Preorder, AvailableFrom: &march}},
			{SKU: "10004", OptionValues: []string{"58"}, PriceCents: 0, Availability: av(availability.InStock)},
			{SKU: "10005", OptionValues: []string{"62"}, PriceCents: 89900, Availability: nil},
		},
	}, now)

	if p.Published {
		t.Error("a draft must not be published")
	}
	if p.Title != "City 3 rot" || p.Vendor != "Beispielrad" || p.ProductType != "Citybike" || len(p.Options) != 1 || len(p.ImageURLs) != 1 {
		t.Errorf("product fields: %+v", p)
	}
	cases := []struct {
		sku      string
		sellable bool
		text     string
	}{
		{"10001", true, "Sofort lieferbar"},
		{"10002", false, "Derzeit nicht lieferbar"},
		{"10003", true, "Vorbestellbar, voraussichtlich ab 15.03.2027"},
		{"10004", false, "Sofort lieferbar"}, // deliverable, but no price: not sellable
		{"10005", false, "Nicht mehr lieferbar"},
	}
	for i, c := range cases {
		v := p.Variants[i]
		if v.SKU != c.sku || v.Sellable != c.sellable || v.DeliveryText != c.text {
			t.Errorf("variant %s: sellable=%v text=%q, want %v %q", v.SKU, v.Sellable, v.DeliveryText, c.sellable, c.text)
		}
	}
	if v := p.Variants[0]; v.Barcode != "4006381333931" || v.PriceCents != 89900 || v.CompareAtCents != 99900 {
		t.Errorf("first variant: %+v", v)
	}

	// An active product is published; nothing has been synced, so the plan
	// is to create it. Projecting again yields no difference.
	active := CatalogProduct{Title: "X", Active: true, Variants: []CatalogVariant{{SKU: "1", PriceCents: 100, Availability: av(availability.InStock)}}}
	first := Project(active, now)
	if !first.Published {
		t.Error("an active product must be published")
	}
	if ops := Diff(nil, first); len(ops) != 1 || ops[0].Kind != CreateProduct {
		t.Errorf("first sync: %+v", ops)
	}
	if ops := Diff(&first, Project(active, now)); len(ops) != 0 {
		t.Errorf("unchanged product has a diff: %+v", ops)
	}
	// The supplier sells out: only that variant changes.
	active.Variants[0].Availability = av(availability.Unavailable)
	ops := Diff(&first, Project(active, now))
	if len(ops) != 1 || ops[0].Kind != UpdateVariant || ops[0].SKU != "1" || len(ops[0].Fields) != 2 {
		t.Errorf("after selling out: %+v", ops)
	}
}
