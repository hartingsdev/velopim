// Package csvfile is a connector for suppliers that deliver a flat CSV file
// with one row per article. Which column means what is configuration, so a
// new supplier with a CSV feed needs no code.
package csvfile

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"velopim/internal/connector"
)

// Target fields a column can be mapped to.
const (
	SKU         = "sku"
	EAN         = "ean"
	MPN         = "mpn"
	Brand       = "brand"
	Name        = "name"
	ModelYear   = "model_year"
	Category    = "category"
	Color       = "color"
	FrameSize   = "frame_size"
	Description = "description"
	Images      = "images"
	Cost        = "cost"
	RRP         = "rrp"
)

// Config describes one supplier's file layout.
type Config struct {
	SourceID string `json:"source_id"`
	// Delimiter is the column separator, usually ";" or ",".
	Delimiter string `json:"delimiter"`
	// DecimalComma is true when prices look like "1.299,00".
	DecimalComma bool `json:"decimal_comma"`
	// Columns maps target fields (see constants) to column headers.
	Columns map[string]string `json:"columns"`
	// ImageSeparator splits several image URLs in one cell; default "|".
	ImageSeparator string `json:"image_separator,omitempty"`
	// AttributeColumns lists headers copied into SupplierItem.Attributes.
	AttributeColumns []string `json:"attribute_columns,omitempty"`
	// Encoding is "utf-8" or "windows-1252". Empty means: UTF-8 when the
	// file is valid UTF-8, otherwise Windows-1252, the usual encoding of
	// files exported from German Windows systems.
	Encoding string `json:"encoding,omitempty"`
}

// Connector implements connector.Connector for a configured CSV layout.
type Connector struct{ cfg Config }

// New validates the configuration.
func New(cfg Config) (*Connector, error) {
	if cfg.SourceID == "" {
		return nil, errors.New("csvfile: source_id is required")
	}
	if len([]rune(cfg.Delimiter)) != 1 {
		return nil, errors.New("csvfile: delimiter must be a single character")
	}
	if cfg.Columns[SKU] == "" {
		return nil, errors.New("csvfile: a column for \"sku\" is required")
	}
	switch cfg.Encoding {
	case "", "utf-8", "windows-1252":
	default:
		return nil, fmt.Errorf("csvfile: unknown encoding %q (utf-8, windows-1252)", cfg.Encoding)
	}
	if cfg.ImageSeparator == "" {
		cfg.ImageSeparator = "|"
	}
	return &Connector{cfg: cfg}, nil
}

func (c *Connector) SourceID() string { return c.cfg.SourceID }

// Parse reads the file. The first row is the header. Every column is kept
// in the raw record, mapped or not.
func (c *Connector) Parse(r io.Reader) (connector.ParseResult, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return connector.ParseResult{}, fmt.Errorf("csvfile: reading file: %w", err)
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if c.cfg.Encoding == "windows-1252" || c.cfg.Encoding == "" && !utf8.Valid(data) {
		data = decodeWindows1252(data)
	}
	cr := csv.NewReader(bytes.NewReader(data))
	cr.Comma = []rune(c.cfg.Delimiter)[0]
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1

	header, err := cr.Read()
	if err != nil {
		return connector.ParseResult{}, fmt.Errorf("csvfile: reading header: %w", err)
	}
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
	}
	skuCol := c.cfg.Columns[SKU]
	if !contains(header, skuCol) {
		return connector.ParseResult{}, fmt.Errorf("csvfile: column %q not found in header", skuCol)
	}

	var res connector.ParseResult
	seen := map[string]bool{}
	for line := 2; ; line++ {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			res.Issues = append(res.Issues, connector.Issue{Message: fmt.Sprintf("Zeile %d: nicht lesbar (%v)", line, err)})
			continue
		}
		if len(row) != len(header) {
			res.Issues = append(res.Issues, connector.Issue{
				Message: fmt.Sprintf("Zeile %d: %d Spalten, die Kopfzeile hat %d", line, len(row), len(header))})
			continue
		}
		fields := make(map[string]string, len(header))
		for i, h := range header {
			fields[h] = strings.TrimSpace(row[i])
		}
		key := fields[skuCol]
		switch {
		case key == "":
			res.Issues = append(res.Issues, connector.Issue{Message: fmt.Sprintf("Zeile %d: keine Artikelnummer", line)})
		case seen[key]:
			res.Issues = append(res.Issues, connector.Issue{Key: key, Message: fmt.Sprintf("Zeile %d: Artikelnummer kommt doppelt vor", line)})
		default:
			seen[key] = true
			res.Records = append(res.Records, connector.RawRecord{Key: key, Fields: fields})
		}
	}
	return res, nil
}

// Normalize maps a raw record onto a SupplierItem and reports quality issues.
func (c *Connector) Normalize(rec connector.RawRecord) (connector.SupplierItem, []connector.Issue) {
	get := func(target string) string { return rec.Fields[c.cfg.Columns[target]] }
	var issues []connector.Issue
	issue := func(field, msg string) {
		issues = append(issues, connector.Issue{Key: rec.Key, Field: field, Message: msg})
	}

	it := connector.SupplierItem{
		SourceID:    c.cfg.SourceID,
		SKU:         rec.Key,
		EAN:         strings.ReplaceAll(get(EAN), " ", ""),
		MPN:         get(MPN),
		Brand:       get(Brand),
		Name:        get(Name),
		Category:    get(Category),
		Color:       get(Color),
		FrameSize:   get(FrameSize),
		Description: get(Description),
	}

	if it.EAN == "" {
		issue(EAN, "EAN fehlt")
	} else if !connector.ValidGTIN(it.EAN) {
		issue(EAN, fmt.Sprintf("EAN %q hat eine falsche Länge oder Prüfziffer", it.EAN))
	}
	if it.Name == "" {
		issue(Name, "Bezeichnung fehlt")
	}

	if v := get(ModelYear); v != "" {
		y, err := strconv.Atoi(v)
		if err != nil || y < 2000 || y > 2100 {
			issue(ModelYear, fmt.Sprintf("Modelljahr %q ist kein plausibles Jahr", v))
		} else {
			it.ModelYear = y
		}
	}

	for target, dst := range map[string]*int64{Cost: &it.CostCents, RRP: &it.RRPCents} {
		v := get(target)
		if v == "" {
			continue
		}
		cents, err := ParseCents(v, c.cfg.DecimalComma)
		if err != nil {
			issue(target, fmt.Sprintf("Preis %q ist keine Zahl", v))
			continue
		}
		*dst = cents
	}
	if it.CostCents > 0 && it.RRPCents > 0 && it.CostCents >= it.RRPCents {
		issue(Cost, "Einkaufspreis liegt nicht unter der UVP")
	}

	for _, u := range strings.Split(get(Images), c.cfg.ImageSeparator) {
		if u = strings.TrimSpace(u); u != "" {
			it.ImageURLs = append(it.ImageURLs, u)
		}
	}

	for _, col := range c.cfg.AttributeColumns {
		if v := rec.Fields[col]; v != "" {
			if it.Attributes == nil {
				it.Attributes = map[string]string{}
			}
			it.Attributes[col] = v
		}
	}
	return it, issues
}

// ParseCents parses a money amount such as "1.299,00 €" or "1299.00".
func ParseCents(s string, decimalComma bool) (int64, error) {
	s = strings.TrimSpace(strings.NewReplacer("€", "", "EUR", "", " ", "", " ", "").Replace(s))
	if decimalComma {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	} else {
		s = strings.ReplaceAll(s, ",", "")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, fmt.Errorf("csvfile: %q is not a money amount", s)
	}
	return int64(math.Round(f * 100)), nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// windows1252 maps the bytes 0x80 to 0x9F, where Windows-1252 differs from
// Latin-1. Unassigned bytes become the replacement character.
var windows1252 = [32]rune{
	0x20AC, 0xFFFD, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021, 0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0xFFFD, 0x017D, 0xFFFD,
	0xFFFD, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014, 0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0xFFFD, 0x017E, 0x0178,
}

// decodeWindows1252 converts Windows-1252 bytes to UTF-8.
func decodeWindows1252(data []byte) []byte {
	out := make([]byte, 0, len(data)+len(data)/8)
	for _, b := range data {
		switch {
		case b < 0x80:
			out = append(out, b)
		case b < 0xA0:
			out = utf8.AppendRune(out, windows1252[b-0x80])
		default:
			out = utf8.AppendRune(out, rune(b)) // identical to Latin-1 here
		}
	}
	return out
}

// Preview reads the header and the first rows of a file, to help mapping
// its columns. encoding is as in Config; the detected encoding is returned.
func Preview(data []byte, delimiter, encoding string, maxRows int) (header []string, rows [][]string, detected string, err error) {
	if len([]rune(delimiter)) != 1 {
		return nil, nil, "", errors.New("csvfile: delimiter must be a single character")
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	detected = "utf-8"
	if encoding == "windows-1252" || encoding == "" && !utf8.Valid(data) {
		data, detected = decodeWindows1252(data), "windows-1252"
	}
	cr := csv.NewReader(bytes.NewReader(data))
	cr.Comma = []rune(delimiter)[0]
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	header, err = cr.Read()
	if err != nil {
		return nil, nil, "", fmt.Errorf("csvfile: reading header: %w", err)
	}
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
	}
	for len(rows) < maxRows {
		row, err := cr.Read()
		if err != nil {
			break
		}
		rows = append(rows, row)
	}
	return header, rows, detected, nil
}
