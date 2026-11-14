package actions

import (
	"reflect"
	"testing"

	"velopim/internal/availability"
	"velopim/internal/connector"
	"velopim/internal/rules"
)

func base() connector.SupplierItem {
	q := 4
	return connector.SupplierItem{
		SourceID: "example", SKU: "100-001", EAN: "4006381333931", Brand: "Beispielrad",
		Name: "Trekking 5.4", ModelYear: 2027, Color: "schwarz", FrameSize: "52",
		CostCents: 104950, RRPCents: 179900,
		ImageURLs:    []string{"a.jpg", "b.jpg"},
		Attributes:   map[string]string{"Antrieb": "Kette"},
		Availability: availability.Availability{Status: availability.InStock, Quantity: &q},
	}
}

func TestProposeNewProduct(t *testing.T) {
	got := Propose(nil, base())
	want := []Proposal{{Type: rules.NewProduct, SKU: "100-001"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestProposeUnchangedIsEmpty(t *testing.T) {
	prev := base()
	if got := Propose(&prev, base()); len(got) != 0 {
		t.Errorf("unchanged item produced proposals: %+v", got)
	}
}

func TestProposeQuantityAloneIsNoProposal(t *testing.T) {
	prev, cur := base(), base()
	q := 1
	cur.Availability.Quantity = &q
	if got := Propose(&prev, cur); len(got) != 0 {
		t.Errorf("a quantity change must not create a proposal: %+v", got)
	}
}

func TestProposeChanges(t *testing.T) {
	tests := []struct {
		name   string
		change func(*connector.SupplierItem)
		want   Proposal
	}{
		{"purchase price", func(i *connector.SupplierItem) { i.CostCents = 109900 },
			Proposal{rules.PriceChange, "100-001", "cost_cents", "104950", "109900"}},
		{"recommended price", func(i *connector.SupplierItem) { i.RRPCents = 189900 },
			Proposal{rules.PriceChange, "100-001", "rrp_cents", "179900", "189900"}},
		{"availability status", func(i *connector.SupplierItem) { i.Availability.Status = availability.Unavailable },
			Proposal{rules.AvailabilityChange, "100-001", "availability.status", "in_stock", "unavailable"}},
		{"name", func(i *connector.SupplierItem) { i.Name = "Trekking 5.4 Herren" },
			Proposal{rules.DescriptionChange, "100-001", "name", "Trekking 5.4", "Trekking 5.4 Herren"}},
		{"image order", func(i *connector.SupplierItem) { i.ImageURLs = []string{"b.jpg", "a.jpg"} },
			Proposal{rules.ImagesChange, "100-001", "image_urls", "a.jpg\nb.jpg", "b.jpg\na.jpg"}},
		{"EAN", func(i *connector.SupplierItem) { i.EAN = "4006381333948" },
			Proposal{rules.AttributeChange, "100-001", "ean", "4006381333931", "4006381333948"}},
		{"model year", func(i *connector.SupplierItem) { i.ModelYear = 2028 },
			Proposal{rules.AttributeChange, "100-001", "model_year", "2027", "2028"}},
		{"attribute removed", func(i *connector.SupplierItem) { i.Attributes = nil },
			Proposal{rules.AttributeChange, "100-001", "attributes.Antrieb", "Kette", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev, cur := base(), base()
			tt.change(&cur)
			got := Propose(&prev, cur)
			if len(got) != 1 || got[0] != tt.want {
				t.Errorf("got %+v, want exactly %+v", got, tt.want)
			}
		})
	}
}

func TestProposeRemoved(t *testing.T) {
	got := ProposeRemoved([]string{"c", "a", "b"}, []string{"b", "d"})
	want := []Proposal{{Type: rules.Deactivation, SKU: "a"}, {Type: rules.Deactivation, SKU: "c"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got := ProposeRemoved([]string{"a"}, []string{"a"}); len(got) != 0 {
		t.Errorf("nothing removed, got %+v", got)
	}
}
