// Package connector defines what every supplier source must provide:
// parsing a delivery into raw records and normalizing a raw record into a
// SupplierItem. Fetching (FTP, API, upload) and persistence live elsewhere.
package connector

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"

	"velopim/internal/availability"
)

// RawRecord is one article exactly as the supplier delivered it.
type RawRecord struct {
	// Key is the supplier's article number; it identifies the record across imports.
	Key    string            `json:"key"`
	Fields map[string]string `json:"fields"`
}

// Hash identifies the content of a record. A new raw version is stored only
// when the hash differs from the previous import.
func (r RawRecord) Hash() string {
	// encoding/json writes map keys in sorted order, so the hash is stable.
	b, _ := json.Marshal(r.Fields)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// SupplierItem is the normalized view of one article of one source.
type SupplierItem struct {
	SourceID    string            `json:"source_id"`
	SKU         string            `json:"sku"` // supplier article number
	EAN         string            `json:"ean,omitempty"`
	MPN         string            `json:"mpn,omitempty"` // manufacturer part number
	Brand       string            `json:"brand,omitempty"`
	Name        string            `json:"name,omitempty"`
	ModelYear   int               `json:"model_year,omitempty"`
	Category    string            `json:"category,omitempty"`
	Color       string            `json:"color,omitempty"`
	FrameSize   string            `json:"frame_size,omitempty"`
	Description string            `json:"description,omitempty"`
	ImageURLs   []string          `json:"image_urls,omitempty"`
	CostCents   int64             `json:"cost_cents,omitempty"` // net purchase price
	RRPCents    int64             `json:"rrp_cents,omitempty"`  // gross recommended retail price
	Attributes  map[string]string `json:"attributes,omitempty"`
	// Availability is filled by the import pipeline from the source's mapping rules.
	Availability availability.Availability `json:"availability"`
}

// Issue is a data-quality finding. Issues do not stop an import; they become tasks.
type Issue struct {
	Key     string `json:"key,omitempty"` // supplier article number, if known
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// ParseResult is the outcome of parsing one delivery.
type ParseResult struct {
	Records []RawRecord
	// Issues lists rows that could not be turned into a record.
	Issues []Issue
}

// Connector is the interface every source implements.
type Connector interface {
	// SourceID is the stable identifier of the source, e.g. "hartje".
	SourceID() string
	// Parse reads one delivery. It fails only when the delivery as a whole
	// is unreadable; bad rows are reported as issues.
	Parse(r io.Reader) (ParseResult, error)
	// Normalize maps a raw record onto the supplier-independent model.
	Normalize(rec RawRecord) (SupplierItem, []Issue)
}

// ValidGTIN reports whether s is a GTIN-8, -12, -13 or -14 with a correct
// check digit.
func ValidGTIN(s string) bool {
	switch len(s) {
	case 8, 12, 13, 14:
	default:
		return false
	}
	sum := 0
	for i := 0; i < len(s); i++ {
		c := s[len(s)-1-i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if i%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return sum%10 == 0
}

// SuspiciousDrop reports whether an import lost so many articles compared
// with the previous one that it is more likely truncated than real. Such an
// import must not deactivate anything without a human looking at it.
func SuspiciousDrop(previous, current int, maxDropPct float64) bool {
	if previous == 0 {
		return false
	}
	return float64(previous-current)/float64(previous)*100 > maxDropPct
}
