package shopify

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveImages checks against the development store how images behave:
// that Shopify fetches them, and what a repeated sync does. Runs only with
// SHOPIFY_LIVE_TEST=1 (see TestLiveProductSet).
func TestLiveImages(t *testing.T) {
	if os.Getenv("SHOPIFY_LIVE_TEST") != "1" {
		t.Skip("SHOPIFY_LIVE_TEST not set")
	}
	ctx := context.Background()
	c, err := NewClient(Config{StoreDomain: os.Getenv("SHOPIFY_STORE_DOMAIN"), ClientID: os.Getenv("SHOPIFY_CLIENT_ID"),
		ClientSecret: os.Getenv("SHOPIFY_CLIENT_SECRET"), WriteEnabled: os.Getenv("SHOPIFY_WRITE_ENABLED") == "true"})
	if err != nil {
		t.Fatal(err)
	}
	const good = "https://upload.wikimedia.org/wikipedia/commons/4/47/PNG_transparency_demonstration_1.png"
	p := Product{Title: "velopim Bildtest", Vendor: "Beispielrad", ProductType: "Test",
		ImageURLs: []string{good, "https://example.test/does-not-exist.jpg"},
		Variants:  []Variant{{SKU: "IMG-1", PriceCents: 1000, Sellable: true, DeliveryText: "Sofort lieferbar"}}}

	created, err := c.SetProduct(ctx, p, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.DeleteProduct(ctx, created.ProductID) })
	if len(created.MediaIDs) != 2 {
		t.Fatalf("media ids after create: %v", created.MediaIDs)
	}

	// Shopify fetches the images afterwards: the reachable one becomes
	// ready, the unreachable one fails.
	states, err := c.WaitForMedia(ctx, created.ProductID, 40*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || states[0].ID != created.MediaIDs[0] || states[0].Status != "READY" || states[1].Status != "FAILED" {
		t.Fatalf("media states: %+v", states)
	}

	// A later sync refers to the ready image by id: Shopify keeps it
	// instead of fetching it again. The failed one is sent again.
	p.Variants[0].PriceCents = 1100
	again, err := c.SetProduct(ctx, p, created.ProductID, map[string]string{good: states[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.MediaIDs) != 2 || again.MediaIDs[0] != states[0].ID {
		t.Errorf("ready image was not kept: before %s, after %v", states[0].ID, again.MediaIDs)
	}
	if again.MediaIDs[1] == created.MediaIDs[1] {
		t.Errorf("failed image was not sent again")
	}

	// Dropping an image from the product removes it in Shopify.
	p.ImageURLs = p.ImageURLs[:1]
	last, err := c.SetProduct(ctx, p, created.ProductID, map[string]string{good: states[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(last.MediaIDs) != 1 || last.MediaIDs[0] != states[0].ID {
		t.Errorf("after dropping an image: %v", last.MediaIDs)
	}
}
