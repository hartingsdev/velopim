package shopify

import (
	"time"

	"velopim/internal/availability"
)

// CatalogProduct is what the projection is computed from: a product of the
// own catalog with the supplier availability of its variants. It carries
// only what may go to Shopify; purchase prices, notes and other internal
// data are not part of it.
type CatalogProduct struct {
	Title           string
	DescriptionHTML string
	Vendor          string
	ProductType     string
	// Active is false for drafts, which Shopify must not publish.
	Active    bool
	Options   []string
	ImageURLs []string
	Variants  []CatalogVariant
}

// CatalogVariant is one variant of a CatalogProduct.
type CatalogVariant struct {
	SKU            string
	EAN            string
	OptionValues   []string
	PriceCents     int64 // 0: no price yet
	CompareAtCents int64
	// Availability is the preferred supplier's current availability; nil
	// when the variant has no supplier article or the article left the feed.
	Availability *availability.Availability
}

// Project computes what Shopify should show for a product. Availability is
// read live from the supplier (docs/decisions/0027): a variant is sellable
// when its supplier can deliver and it has a price.
func Project(p CatalogProduct, now time.Time) Product {
	out := Product{
		Title: p.Title, DescriptionHTML: p.DescriptionHTML, Vendor: p.Vendor, ProductType: p.ProductType,
		Published: p.Active, Options: append([]string{}, p.Options...), ImageURLs: append([]string{}, p.ImageURLs...),
		Variants: make([]Variant, 0, len(p.Variants)),
	}
	for _, v := range p.Variants {
		display := availability.Display{Sellable: false, Text: "Nicht mehr lieferbar"}
		if v.Availability != nil {
			display = availability.Derive(*v.Availability, now)
		}
		out.Variants = append(out.Variants, Variant{
			SKU: v.SKU, Barcode: v.EAN, OptionValues: append([]string{}, v.OptionValues...),
			PriceCents: v.PriceCents, CompareAtCents: v.CompareAtCents,
			Sellable: display.Sellable && v.PriceCents > 0, DeliveryText: display.Text,
		})
	}
	return out
}
