package catalog

import (
	"reflect"
	"strings"
	"testing"

	"velopim/internal/connector"
)

func item(sku, name, color, size string, attrs ...string) connector.SupplierItem {
	it := connector.SupplierItem{SKU: sku, Brand: "Beispielrad", Name: name, ModelYear: 2027, Color: color, FrameSize: size}
	for i := 0; i+1 < len(attrs); i += 2 {
		if it.Attributes == nil {
			it.Attributes = map[string]string{}
		}
		it.Attributes[attrs[i]] = attrs[i+1]
	}
	return it
}

// summary renders products compactly: "Title [Options] sku=values ...".
func summary(ps []Product) []string {
	var out []string
	for _, p := range ps {
		s := p.Title + " [" + strings.Join(p.OptionNames, ",") + "]"
		for _, v := range p.Variants {
			s += " " + v.SKU + "=" + strings.Join(v.OptionValues, "/")
		}
		out = append(out, s)
	}
	return out
}

func TestGroup(t *testing.T) {
	trekking := []connector.SupplierItem{
		item("4", "Trekking 5.4", "silber", "56", "Rahmenform", "Wave", "Antrieb", "Kette"),
		item("1", "Trekking 5.4", "schwarz", "52", "Rahmenform", "Herren", "Antrieb", "Kette"),
		item("3", "Trekking 5.4", "silber", "52", "Rahmenform", "Wave", "Antrieb", "Kette"),
		item("2", "Trekking 5.4", "schwarz", "56", "Rahmenform", "Herren", "Antrieb", "Kette"),
	}
	model := ModelKey{Brand: "Beispielrad", Name: "Trekking 5.4", ModelYear: 2027}
	frameShape := Axis("attributes.Rahmenform")
	drive := Axis("attributes.Antrieb")

	cases := []struct {
		name       string
		items      []connector.SupplierItem
		settings   Settings
		want       []string
		wantIssues []string // substrings, over all products
	}{
		{
			name:     "default: one product per model, color and frame size as options",
			items:    trekking,
			settings: Settings{Default: DefaultConfig},
			want:     []string{"Trekking 5.4 [Farbe,Rahmenhöhe] 1=schwarz/52 2=schwarz/56 3=silber/52 4=silber/56"},
		},
		{
			name:     "split by color",
			items:    trekking,
			settings: Settings{Default: Config{SplitBy: []Axis{AxisColor}, Options: DefaultConfig.Options}},
			want: []string{
				"Trekking 5.4 schwarz [Rahmenhöhe] 1=52 2=56",
				"Trekking 5.4 silber [Rahmenhöhe] 3=52 4=56",
			},
		},
		{
			name:  "split by a supplier attribute for one model only",
			items: append(append([]connector.SupplierItem{}, trekking...), item("9", "City 3", "rot", "50"), item("8", "City 3", "blau", "50")),
			settings: Settings{Default: DefaultConfig, PerModel: map[ModelKey]Config{
				model: {SplitBy: []Axis{frameShape}, Options: DefaultConfig.Options}}},
			want: []string{
				"City 3 [Farbe,Rahmenhöhe] 8=blau/50 9=rot/50",
				"Trekking 5.4 Herren [Farbe,Rahmenhöhe] 1=schwarz/52 2=schwarz/56",
				"Trekking 5.4 Wave [Farbe,Rahmenhöhe] 3=silber/52 4=silber/56",
			},
		},
		{
			name:     "split by two axes",
			items:    trekking,
			settings: Settings{Default: Config{SplitBy: []Axis{frameShape, AxisColor}, Options: []Axis{AxisFrameSize}}},
			want: []string{
				"Trekking 5.4 Herren schwarz [Rahmenhöhe] 1=52 2=56",
				"Trekking 5.4 Wave silber [Rahmenhöhe] 3=52 4=56",
			},
		},
		{
			name:     "attribute as option; axis without any value is dropped",
			items:    []connector.SupplierItem{item("1", "City 3", "", "50", "Antrieb", "Kette"), item("2", "City 3", "", "50", "Antrieb", "Riemen")},
			settings: Settings{Default: Config{Options: []Axis{AxisColor, AxisFrameSize, drive}}},
			want:     []string{"City 3 [Rahmenhöhe,Antrieb] 1=50/Kette 2=50/Riemen"},
		},
		{
			name:     "different model years are different products",
			items:    []connector.SupplierItem{item("1", "City 3", "rot", "50"), {SKU: "2", Brand: "Beispielrad", Name: "City 3", ModelYear: 2028, Color: "rot", FrameSize: "50"}},
			settings: Settings{Default: DefaultConfig},
			want:     []string{"City 3 [Farbe,Rahmenhöhe] 1=rot/50", "City 3 [Farbe,Rahmenhöhe] 2=rot/50"},
		},
		{
			name:       "indistinguishable variants are an issue",
			items:      []connector.SupplierItem{item("1", "City 3", "rot", "50", "Antrieb", "Kette"), item("2", "City 3", "rot", "50", "Antrieb", "Riemen")},
			settings:   Settings{Default: DefaultConfig},
			want:       []string{"City 3 [Farbe,Rahmenhöhe] 1=rot/50 2=rot/50"},
			wantIssues: []string{"Artikel 1 und 2 sind nicht unterscheidbar (Farbe: rot, Rahmenhöhe: 50)"},
		},
		{
			name:       "missing option value is an issue",
			items:      []connector.SupplierItem{item("1", "City 3", "rot", "50"), item("2", "City 3", "", "54")},
			settings:   Settings{Default: DefaultConfig},
			want:       []string{"City 3 [Farbe,Rahmenhöhe] 2=/54 1=rot/50"},
			wantIssues: []string{`Artikel 2 hat keinen Wert für "Farbe"`},
		},
		{
			name:       "missing split value is an issue and a product of its own",
			items:      []connector.SupplierItem{item("1", "City 3", "rot", "50"), item("2", "City 3", "", "50")},
			settings:   Settings{Default: Config{SplitBy: []Axis{AxisColor}, Options: DefaultConfig.Options}},
			want:       []string{"City 3 [Rahmenhöhe] 2=50", "City 3 rot [Rahmenhöhe] 1=50"},
			wantIssues: []string{`ohne Wert für das Trennmerkmal "Farbe"`},
		},
		{
			name:       "more options than Shopify allows",
			items:      []connector.SupplierItem{item("1", "City 3", "rot", "50", "Antrieb", "Kette", "Rahmenform", "Wave")},
			settings:   Settings{Default: Config{Options: []Axis{AxisColor, AxisFrameSize, drive, frameShape}}},
			want:       []string{"City 3 [Farbe,Rahmenhöhe,Antrieb,Rahmenform] 1=rot/50/Kette/Wave"},
			wantIssues: []string{"4 Optionen, Shopify erlaubt 3"},
		},
		{
			name:       "no options: a single item is fine, several are not",
			items:      []connector.SupplierItem{item("1", "Solo", "", ""), item("2", "Duo", "", ""), item("3", "Duo", "", "")},
			settings:   Settings{Default: DefaultConfig},
			want:       []string{"Duo [] 2= 3=", "Solo [] 1="},
			wantIssues: []string{"Artikel 2 und 3 sind nicht unterscheidbar (keine Option unterscheidet sie)"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Group(c.items, c.settings)
			if s := summary(got); !reflect.DeepEqual(s, c.want) {
				t.Errorf("products:\n got  %q\n want %q", s, c.want)
			}
			var issues []string
			for _, p := range got {
				issues = append(issues, p.Issues...)
			}
			if len(issues) != len(c.wantIssues) {
				t.Fatalf("issues = %q, want %d", issues, len(c.wantIssues))
			}
			for i, want := range c.wantIssues {
				if !strings.Contains(issues[i], want) {
					t.Errorf("issue %d = %q, want it to contain %q", i, issues[i], want)
				}
			}
		})
	}
}

func TestGroupIsDeterministic(t *testing.T) {
	items := []connector.SupplierItem{
		item("1", "A", "rot", "50"), item("2", "A", "blau", "50"), item("3", "B", "rot", "50"), item("4", "C", "rot", "50"),
	}
	s := Settings{Default: Config{SplitBy: []Axis{AxisColor}, Options: DefaultConfig.Options}}
	first := summary(Group(items, s))
	for range 20 {
		if again := summary(Group(items, s)); !reflect.DeepEqual(again, first) {
			t.Fatalf("order changed: %q vs %q", again, first)
		}
	}
}
