package catalog

import "testing"

func TestUsagePolicies(t *testing.T) {
	ps := UsagePolicies{
		{SourceID: "hartje", Brand: "", Group: GroupImages, Policy: PolicyInternalOnly},
		{SourceID: "hartje", Brand: "Victoria", Group: GroupImages, Policy: PolicyUse},
		{SourceID: "hartje", Brand: "Conway", Group: GroupTexts, Policy: PolicyIgnore},
		{SourceID: "other", Brand: "", Group: GroupTexts, Policy: PolicyInternalOnly},
	}
	cases := []struct {
		name                 string
		source, brand, group string
		want                 string
	}{
		{"nothing configured means use", "hartje", "Victoria", GroupPrices, PolicyUse},
		{"source-wide entry", "hartje", "Conway", GroupImages, PolicyInternalOnly},
		{"brand entry wins over source-wide", "hartje", "Victoria", GroupImages, PolicyUse},
		{"brand is compared without case", "hartje", "VICTORIA", GroupImages, PolicyUse},
		{"brand entry without source-wide one", "hartje", "Conway", GroupTexts, PolicyIgnore},
		{"other brand is not affected", "hartje", "Victoria", GroupTexts, PolicyUse},
		{"other source is separate", "other", "Victoria", GroupTexts, PolicyInternalOnly},
		{"unknown source", "nobody", "Victoria", GroupImages, PolicyUse},
	}
	for _, c := range cases {
		if got := ps.For(c.source, c.brand, c.group); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	// A brand entry wins no matter where it stands in the list.
	reversed := UsagePolicies{ps[1], ps[0]}
	if got := reversed.For("hartje", "Victoria", GroupImages); got != PolicyUse {
		t.Errorf("order must not matter: got %q", got)
	}
}

func TestGroupOfField(t *testing.T) {
	for field, want := range map[string]string{
		"image_urls": GroupImages, "name": GroupTexts, "description": GroupTexts, "cost_cents": GroupPrices,
		"rrp_cents": GroupPrices, "ean": GroupAttributes, "color": GroupAttributes, "attributes.Antrieb": GroupAttributes,
		"availability.status": GroupAttributes,
	} {
		if got := GroupOfField(field); got != want {
			t.Errorf("%s: got %q, want %q", field, got, want)
		}
	}
}
