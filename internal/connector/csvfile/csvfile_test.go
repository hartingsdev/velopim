package csvfile

import (
	"os"
	"strings"
	"testing"

	"velopim/internal/connector"
)

// Layout of testdata/example_supplier.csv. This is an invented file, not a
// real supplier format.
var exampleConfig = Config{
	SourceID:     "example",
	Delimiter:    ";",
	DecimalComma: true,
	Columns: map[string]string{
		SKU: "Artikelnr", EAN: "EAN", Brand: "Marke", Name: "Bezeichnung", ModelYear: "Modelljahr",
		Color: "Farbe", FrameSize: "Rahmenhoehe", Cost: "EK", RRP: "UVP", Images: "Bilder",
	},
	AttributeColumns: []string{"Antrieb"},
}

func parseExample(t *testing.T) (*Connector, connector.ParseResult) {
	t.Helper()
	c, err := New(exampleConfig)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open("../../../testdata/example_supplier.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	res, err := c.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	return c, res
}

func TestParse(t *testing.T) {
	_, res := parseExample(t)
	if len(res.Records) != 3 {
		t.Fatalf("got %d records, want 3", len(res.Records))
	}
	if len(res.Issues) != 3 {
		t.Fatalf("got %d issues, want 3 (no number, duplicate, short row): %+v", len(res.Issues), res.Issues)
	}
	first := res.Records[0]
	if first.Key != "100-001" || first.Fields["Bestand"] != "4" || first.Fields["Antrieb"] != "Kette" {
		t.Errorf("raw record must keep every column unchanged: %+v", first)
	}
}

func TestParseStripsBOMAndRejectsMissingKeyColumn(t *testing.T) {
	c, _ := New(exampleConfig)
	res, err := c.Parse(strings.NewReader("\xef\xbb\xbfArtikelnr;EAN\n1;2\n"))
	if err != nil || len(res.Records) != 1 || res.Records[0].Key != "1" {
		t.Errorf("BOM not handled: %+v, %v", res, err)
	}
	if _, err := c.Parse(strings.NewReader("Nummer;EAN\n1;2\n")); err == nil {
		t.Error("expected an error when the article number column is missing")
	}
}

func TestNormalize(t *testing.T) {
	c, res := parseExample(t)

	it, issues := c.Normalize(res.Records[0])
	if len(issues) != 0 {
		t.Errorf("clean row produced issues: %+v", issues)
	}
	if it.SourceID != "example" || it.SKU != "100-001" || it.EAN != "4006381333931" || it.Brand != "Beispielrad" ||
		it.ModelYear != 2027 || it.Color != "schwarz matt" || it.FrameSize != "52" ||
		it.CostCents != 104950 || it.RRPCents != 179900 || len(it.ImageURLs) != 2 || it.Attributes["Antrieb"] != "Kette" {
		t.Errorf("unexpected item: %+v", it)
	}

	_, issues = c.Normalize(res.Records[1])
	if len(issues) != 1 || issues[0].Field != EAN {
		t.Errorf("want one issue for the wrong EAN check digit, got %+v", issues)
	}

	it, issues = c.Normalize(res.Records[2])
	fields := map[string]bool{}
	for _, i := range issues {
		fields[i.Field] = true
		if i.Key != "100-003" {
			t.Errorf("issue without article number: %+v", i)
		}
	}
	if !fields[EAN] || !fields[ModelYear] || !fields[RRP] || len(issues) != 3 {
		t.Errorf("want issues for EAN, model year and RRP, got %+v", issues)
	}
	if it.ModelYear != 0 || it.RRPCents != 0 || it.CostCents != 104950 {
		t.Errorf("bad values must stay empty, good ones must be kept: %+v", it)
	}
}

func TestRawHashIsStableAndContentSensitive(t *testing.T) {
	a := connector.RawRecord{Key: "1", Fields: map[string]string{"a": "1", "b": "2"}}
	b := connector.RawRecord{Key: "1", Fields: map[string]string{"b": "2", "a": "1"}}
	c := connector.RawRecord{Key: "1", Fields: map[string]string{"a": "1", "b": "3"}}
	if a.Hash() != b.Hash() {
		t.Error("hash depends on map order")
	}
	if a.Hash() == c.Hash() {
		t.Error("hash ignores a changed value")
	}
}

func TestParseCents(t *testing.T) {
	tests := []struct {
		in      string
		comma   bool
		want    int64
		wantErr bool
	}{
		{"1.299,00", true, 129900, false},
		{"1.299,00 €", true, 129900, false},
		{"899,5", true, 89950, false},
		{"1299.00", false, 129900, false},
		{"1,299.00", false, 129900, false},
		{"0,29", true, 29, false},
		{"abc", true, 0, true},
		{"-5,00", true, 0, true},
		{"", true, 0, true},
	}
	for _, tt := range tests {
		got, err := ParseCents(tt.in, tt.comma)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ParseCents(%q, %v) = %d, %v; want %d, error=%v", tt.in, tt.comma, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestNewValidatesConfig(t *testing.T) {
	for name, cfg := range map[string]Config{
		"no source":    {Delimiter: ";", Columns: map[string]string{SKU: "x"}},
		"no delimiter": {SourceID: "s", Columns: map[string]string{SKU: "x"}},
		"no sku":       {SourceID: "s", Delimiter: ";"},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestValidGTINAndSuspiciousDrop(t *testing.T) {
	for s, want := range map[string]bool{
		"4006381333931": true, "4006381333932": false, "73513537": true,
		"036000291452": true, "123": false, "40063813339AB": false, "": false,
	} {
		if got := connector.ValidGTIN(s); got != want {
			t.Errorf("ValidGTIN(%q) = %v, want %v", s, got, want)
		}
	}
	if !connector.SuspiciousDrop(1000, 600, 20) || connector.SuspiciousDrop(1000, 950, 20) || connector.SuspiciousDrop(0, 0, 20) {
		t.Error("SuspiciousDrop gives wrong results")
	}
}
