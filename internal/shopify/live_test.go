package shopify

import (
	"context"
	"os"
	"testing"
)

// TestLiveProductSet runs the whole life of a product against the real
// development store: create as draft, repeat, change, publish, delete. It
// only runs with SHOPIFY_LIVE_TEST=1 and the Shopify variables set, e.g.
//
//	set -a; . ./.env; set +a
//	SHOPIFY_LIVE_TEST=1 SHOPIFY_WRITE_ENABLED=true go test ./internal/shopify -run Live -v
//
// The client refuses to write to anything but a development store.
func TestLiveProductSet(t *testing.T) {
	if os.Getenv("SHOPIFY_LIVE_TEST") != "1" {
		t.Skip("SHOPIFY_LIVE_TEST not set")
	}
	ctx := context.Background()
	c, err := NewClient(Config{StoreDomain: os.Getenv("SHOPIFY_STORE_DOMAIN"), ClientID: os.Getenv("SHOPIFY_CLIENT_ID"),
		ClientSecret: os.Getenv("SHOPIFY_CLIENT_SECRET"), WriteEnabled: os.Getenv("SHOPIFY_WRITE_ENABLED") == "true"})
	if err != nil {
		t.Fatal(err)
	}
	p := Product{Title: "velopim Livetest", DescriptionHTML: "<p>Automatischer Test, kann gelöscht werden</p>", Vendor: "Beispielrad",
		ProductType: "Test", Options: []string{"Rahmenhöhe"},
		Variants: []Variant{
			{SKU: "LIVE-1", Barcode: "4006381333931", OptionValues: []string{"50"}, PriceCents: 89900, CompareAtCents: 99900, Sellable: true, DeliveryText: "Sofort lieferbar"},
			{SKU: "LIVE-2", OptionValues: []string{"54"}, PriceCents: 89900, Sellable: false, DeliveryText: "Derzeit nicht lieferbar"},
		}}

	created, err := c.SetProduct(ctx, p, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.DeleteProduct(ctx, created.ProductID); err != nil {
			t.Errorf("cleanup: product %s was not deleted: %v", created.ProductID, err)
		}
	})
	if created.Status != "DRAFT" || !created.AvailableForSale["LIVE-1"] || created.AvailableForSale["LIVE-2"] {
		t.Errorf("created: status %s, available %v", created.Status, created.AvailableForSale)
	}

	// The same input again changes nothing and creates no second product.
	again, err := c.SetProduct(ctx, p, created.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	if again.ProductID != created.ProductID || again.VariantIDs["LIVE-1"] != created.VariantIDs["LIVE-1"] {
		t.Errorf("repeat changed ids: %+v vs %+v", again, created)
	}

	// Availability flips, a variant is added, the product is published.
	p.Published = true
	p.Variants[0].Sellable, p.Variants[1].Sellable = false, true
	p.Variants = append(p.Variants, Variant{SKU: "LIVE-3", OptionValues: []string{"58"}, PriceCents: 79900, Sellable: true, DeliveryText: "Sofort lieferbar"})
	changed, err := c.SetProduct(ctx, p, created.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Status != "ACTIVE" || changed.AvailableForSale["LIVE-1"] || !changed.AvailableForSale["LIVE-2"] || len(changed.VariantIDs) != 3 ||
		changed.VariantIDs["LIVE-1"] != created.VariantIDs["LIVE-1"] {
		t.Errorf("changed: status %s, available %v, variants %v", changed.Status, changed.AvailableForSale, changed.VariantIDs)
	}

	// A variant missing from the input is removed.
	p.Variants = p.Variants[:2]
	removed, err := c.SetProduct(ctx, p, created.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.VariantIDs) != 2 {
		t.Errorf("after removing a variant: %v", removed.VariantIDs)
	}

	// What Shopify stored: delivery text, price, barcode.
	var out struct {
		Product struct {
			Variants struct {
				Nodes []struct {
					SKU            string
					Price          string
					CompareAtPrice *string
					Barcode        *string
					Metafield      *struct{ Value string }
				}
			}
		}
	}
	err = c.Query(ctx, `query($id: ID!) { product(id: $id) { variants(first: 10) { nodes {
		sku price compareAtPrice barcode metafield(namespace: "velopim", key: "delivery_time") { value } } } } }`,
		map[string]any{"id": created.ProductID}, &out)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range out.Product.Variants.Nodes {
		t.Logf("stored variant: %+v metafield=%v", v, v.Metafield)
		if v.Metafield == nil || v.Metafield.Value == "" {
			t.Errorf("variant %s has no delivery text", v.SKU)
		}
		if v.SKU == "LIVE-1" && (v.Price != "899.00" || v.CompareAtPrice == nil || *v.CompareAtPrice != "999.00" || v.Barcode == nil || *v.Barcode != "4006381333931") {
			t.Errorf("LIVE-1 stored as %+v", v)
		}
	}
}
