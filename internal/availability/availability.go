// Package availability holds the supplier-independent availability model,
// the configurable mapping from raw supplier fields onto it, and the
// derivation of what the shop shows. It is pure.
package availability

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Status is the supplier-independent availability status.
type Status string

const (
	InStock      Status = "in_stock"     // at the supplier, ships right away
	Orderable    Status = "orderable"    // not in stock, can be ordered with a lead time
	Preorder     Status = "preorder"     // announced, available from a date
	OnRequest    Status = "on_request"   // must be clarified with the supplier
	Unavailable  Status = "unavailable"  // currently not obtainable
	Discontinued Status = "discontinued" // will not come back
	Unknown      Status = "unknown"      // no mapping rule matched; needs a task
)

// Availability is the normalized availability of one supplier item.
type Availability struct {
	Status        Status     `json:"status"`
	LeadMinDays   *int       `json:"lead_min_days,omitempty"`
	LeadMaxDays   *int       `json:"lead_max_days,omitempty"`
	AvailableFrom *time.Time `json:"available_from,omitempty"`
	Quantity      *int       `json:"quantity,omitempty"`
	Source        string     `json:"source"`
	AsOf          time.Time  `json:"as_of"`
	// MappingRule is the name of the rule that produced this value.
	MappingRule string `json:"mapping_rule,omitempty"`
}

// Op is a test on one raw supplier field.
type Op string

const (
	Eq       Op = "eq"        // equal, case-insensitive
	Contains Op = "contains"  // substring, case-insensitive
	Empty    Op = "empty"     // missing or blank
	NotEmpty Op = "not_empty" // present and not blank
	GT       Op = "gt"        // numeric greater than
	LTE      Op = "lte"       // numeric less than or equal
)

// Condition tests one raw field.
type Condition struct {
	Field string `json:"field"`
	Op    Op     `json:"op"`
	Value string `json:"value,omitempty"`
}

// MappingRule turns raw supplier fields into an availability. Rules are
// evaluated in order; the first whose conditions all hold wins.
type MappingRule struct {
	Name        string      `json:"name"`
	When        []Condition `json:"when"`
	Status      Status      `json:"status"`
	LeadMinDays *int        `json:"lead_min_days,omitempty"`
	LeadMaxDays *int        `json:"lead_max_days,omitempty"`
	// QuantityField names the raw field holding the stock quantity.
	QuantityField string `json:"quantity_field,omitempty"`
	// DateField names the raw field holding the expected date.
	DateField string `json:"date_field,omitempty"`
	// DateLayout is the Go time layout of DateField, e.g. "02.01.2006".
	DateLayout string `json:"date_layout,omitempty"`
}

// Map applies the mapping rules of a source to the raw fields of one item.
// When no rule matches the status is Unknown.
func Map(raw map[string]string, rules []MappingRule, source string, asOf time.Time) Availability {
	av := Availability{Status: Unknown, Source: source, AsOf: asOf}
	for _, r := range rules {
		if !holds(raw, r.When) {
			continue
		}
		av.Status = r.Status
		av.MappingRule = r.Name
		av.LeadMinDays = r.LeadMinDays
		av.LeadMaxDays = r.LeadMaxDays
		if r.QuantityField != "" {
			if n, ok := number(raw[r.QuantityField]); ok {
				q := int(n)
				av.Quantity = &q
			}
		}
		if r.DateField != "" && r.DateLayout != "" {
			if d, err := time.Parse(r.DateLayout, strings.TrimSpace(raw[r.DateField])); err == nil {
				av.AvailableFrom = &d
			}
		}
		return av
	}
	return av
}

func holds(raw map[string]string, conds []Condition) bool {
	for _, c := range conds {
		v := strings.TrimSpace(raw[c.Field])
		switch c.Op {
		case Eq:
			if !strings.EqualFold(v, c.Value) {
				return false
			}
		case Contains:
			if !strings.Contains(strings.ToLower(v), strings.ToLower(c.Value)) {
				return false
			}
		case Empty:
			if v != "" {
				return false
			}
		case NotEmpty:
			if v == "" {
				return false
			}
		case GT, LTE:
			n, ok1 := number(v)
			want, ok2 := number(c.Value)
			if !ok1 || !ok2 {
				return false
			}
			if c.Op == GT && !(n > want) || c.Op == LTE && !(n <= want) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func number(s string) (float64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	n, err := strconv.ParseFloat(s, 64)
	return n, err == nil
}

// Display is what the shop shows for an availability.
type Display struct {
	// Sellable reports whether customers may buy the item.
	Sellable bool
	// Text is the delivery-time text for the storefront metafield.
	Text string
}

// Derive computes the storefront display. now decides whether a preorder
// date is still in the future.
func Derive(av Availability, now time.Time) Display {
	switch av.Status {
	case InStock:
		if t := leadText(av); t != "" {
			return Display{true, "Lieferzeit " + t}
		}
		return Display{true, "Sofort lieferbar"}
	case Orderable:
		if t := leadText(av); t != "" {
			return Display{true, "Lieferzeit " + t}
		}
		return Display{true, "Bestellware, Lieferzeit auf Anfrage"}
	case Preorder:
		if av.AvailableFrom != nil && av.AvailableFrom.After(now) {
			return Display{true, "Vorbestellbar, voraussichtlich ab " + av.AvailableFrom.Format("02.01.2006")}
		}
		return Display{true, "Vorbestellbar"}
	case OnRequest:
		return Display{false, "Verfügbarkeit auf Anfrage"}
	case Unavailable:
		return Display{false, "Derzeit nicht lieferbar"}
	case Discontinued:
		return Display{false, "Nicht mehr lieferbar"}
	}
	return Display{false, "Verfügbarkeit auf Anfrage"}
}

func leadText(av Availability) string {
	lo, hi := av.LeadMinDays, av.LeadMaxDays
	switch {
	case lo != nil && hi != nil && *lo != *hi:
		return fmt.Sprintf("%d–%d Werktage", *lo, *hi)
	case hi != nil:
		return days(*hi)
	case lo != nil:
		return "ab " + days(*lo)
	}
	return ""
}

func days(n int) string {
	if n == 1 {
		return "1 Werktag"
	}
	return fmt.Sprintf("%d Werktage", n)
}
