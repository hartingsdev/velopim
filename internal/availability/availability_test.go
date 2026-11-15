package availability

import (
	"testing"
	"time"
)

func ptr(n int) *int { return &n }

// Example mapping for an imaginary supplier feed with the columns
// "bestand", "status" and "liefertermin". Real supplier mappings are
// configuration, not code.
var exampleRules = []MappingRule{
	{Name: "discontinued", When: []Condition{{Field: "status", Op: Contains, Value: "auslauf"}, {Field: "bestand", Op: LTE, Value: "0"}},
		Status: Discontinued},
	{Name: "in stock", When: []Condition{{Field: "bestand", Op: GT, Value: "0"}},
		Status: InStock, LeadMinDays: ptr(3), LeadMaxDays: ptr(5), QuantityField: "bestand"},
	{Name: "preorder with date", When: []Condition{{Field: "liefertermin", Op: NotEmpty}},
		Status: Preorder, DateField: "liefertermin", DateLayout: "02.01.2006"},
	{Name: "on request", When: []Condition{{Field: "status", Op: Eq, Value: "Anfrage"}},
		Status: OnRequest},
	{Name: "no stock, no date", When: []Condition{{Field: "bestand", Op: LTE, Value: "0"}, {Field: "liefertermin", Op: Empty}},
		Status: Unavailable},
}

func TestMap(t *testing.T) {
	asOf := time.Date(2026, 11, 14, 6, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		raw      map[string]string
		want     Status
		wantRule string
	}{
		{"stock on hand", map[string]string{"bestand": "12"}, InStock, "in stock"},
		{"decimal comma quantity", map[string]string{"bestand": "2,0"}, InStock, "in stock"},
		{"discontinued and sold out", map[string]string{"bestand": "0", "status": "Auslaufartikel"}, Discontinued, "discontinued"},
		{"discontinued but still in stock sells off", map[string]string{"bestand": "3", "status": "Auslaufartikel"}, InStock, "in stock"},
		{"expected date", map[string]string{"bestand": "0", "liefertermin": "15.03.2027"}, Preorder, "preorder with date"},
		{"on request", map[string]string{"bestand": "", "status": "anfrage"}, OnRequest, "on request"},
		{"sold out", map[string]string{"bestand": "0"}, Unavailable, "no stock, no date"},
		{"nothing matches", map[string]string{"bestand": "viele"}, Unknown, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Map(tt.raw, exampleRules, "example", asOf)
			if got.Status != tt.want || got.MappingRule != tt.wantRule {
				t.Errorf("got status=%s rule=%q, want status=%s rule=%q", got.Status, got.MappingRule, tt.want, tt.wantRule)
			}
			if got.Source != "example" || !got.AsOf.Equal(asOf) {
				t.Errorf("source or as-of not carried over: %+v", got)
			}
		})
	}

	got := Map(map[string]string{"bestand": "12"}, exampleRules, "example", asOf)
	if got.Quantity == nil || *got.Quantity != 12 || *got.LeadMinDays != 3 || *got.LeadMaxDays != 5 {
		t.Errorf("quantity or lead time missing: %+v", got)
	}
	got = Map(map[string]string{"bestand": "0", "liefertermin": "15.03.2027"}, exampleRules, "example", asOf)
	if got.AvailableFrom == nil || got.AvailableFrom.Format("2006-01-02") != "2027-03-15" {
		t.Errorf("date not parsed: %+v", got)
	}
}

func TestDerive(t *testing.T) {
	now := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	future := now.AddDate(0, 4, 1)
	past := now.AddDate(0, -1, 0)
	tests := []struct {
		name     string
		av       Availability
		sellable bool
		text     string
	}{
		{"in stock without lead time", Availability{Status: InStock}, true, "Sofort lieferbar"},
		{"in stock with range", Availability{Status: InStock, LeadMinDays: ptr(3), LeadMaxDays: ptr(5)}, true, "Lieferzeit 3–5 Werktage"},
		{"single day", Availability{Status: InStock, LeadMinDays: ptr(1), LeadMaxDays: ptr(1)}, true, "Lieferzeit 1 Werktag"},
		{"orderable with minimum only", Availability{Status: Orderable, LeadMinDays: ptr(10)}, true, "Lieferzeit ab 10 Werktage"},
		{"orderable without lead time", Availability{Status: Orderable}, true, "Bestellware, Lieferzeit auf Anfrage"},
		{"preorder with future date", Availability{Status: Preorder, AvailableFrom: &future}, true, "Vorbestellbar, voraussichtlich ab 15.03.2027"},
		{"preorder with passed date", Availability{Status: Preorder, AvailableFrom: &past}, true, "Vorbestellbar"},
		{"on request", Availability{Status: OnRequest}, false, "Verfügbarkeit auf Anfrage"},
		{"unavailable", Availability{Status: Unavailable}, false, "Derzeit nicht lieferbar"},
		{"discontinued", Availability{Status: Discontinued}, false, "Nicht mehr lieferbar"},
		{"unknown is never sellable", Availability{Status: Unknown}, false, "Verfügbarkeit auf Anfrage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Derive(tt.av, now)
			if got.Sellable != tt.sellable || got.Text != tt.text {
				t.Errorf("got %+v, want sellable=%v text=%q", got, tt.sellable, tt.text)
			}
		})
	}
}

func TestValidateRules(t *testing.T) {
	one, two := 1, 2
	cases := []struct {
		name  string
		rule  MappingRule
		valid bool
	}{
		{"plain rule", MappingRule{Name: "x", Status: InStock, When: []Condition{{Field: "Bestand", Op: GT, Value: "0"}}}, true},
		{"no conditions", MappingRule{Name: "rest", Status: Unavailable}, true},
		{"with date", MappingRule{Status: Preorder, DateField: "Termin", DateLayout: "02.01.2006"}, true},
		{"lead time", MappingRule{Status: Orderable, LeadMinDays: &one, LeadMaxDays: &two}, true},
		{"unknown status", MappingRule{Status: "soon"}, false},
		{"status unknown is not a target", MappingRule{Status: Unknown}, false},
		{"unknown operator", MappingRule{Status: InStock, When: []Condition{{Field: "a", Op: "like"}}}, false},
		{"condition without field", MappingRule{Status: InStock, When: []Condition{{Op: Empty}}}, false},
		{"numeric operator without number", MappingRule{Status: InStock, When: []Condition{{Field: "a", Op: GT, Value: "viel"}}}, false},
		{"lead time reversed", MappingRule{Status: Orderable, LeadMinDays: &two, LeadMaxDays: &one}, false},
		{"date field without layout", MappingRule{Status: Preorder, DateField: "Termin"}, false},
	}
	for _, c := range cases {
		if err := ValidateRules([]MappingRule{c.rule}); (err == nil) != c.valid {
			t.Errorf("%s: ValidateRules = %v, want valid=%v", c.name, err, c.valid)
		}
	}
}
