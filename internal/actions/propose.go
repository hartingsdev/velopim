// Package actions turns differences between two imports into proposed
// actions. Deciding whether a proposal is applied automatically is the job
// of the rules package; this package only detects what changed. It is pure.
package actions

import (
	"sort"
	"strconv"
	"strings"

	"velopim/internal/connector"
	"velopim/internal/rules"
)

// Proposal is one detected change of one supplier item.
type Proposal struct {
	Type rules.ActionType `json:"type"`
	SKU  string           `json:"sku"`
	// Field is the changed field; empty for new products and deactivations.
	Field string `json:"field,omitempty"`
	Old   string `json:"old,omitempty"`
	New   string `json:"new,omitempty"`
}

// Propose compares the previous and the current state of one supplier item.
// prev is nil when the item is new. Quantity changes are not proposals; only
// a change of the availability status is (see architecture decision 8).
func Propose(prev *connector.SupplierItem, cur connector.SupplierItem) []Proposal {
	if prev == nil {
		return []Proposal{{Type: rules.NewProduct, SKU: cur.SKU}}
	}
	var out []Proposal
	add := func(t rules.ActionType, field, old, new string) {
		if old != new {
			out = append(out, Proposal{Type: t, SKU: cur.SKU, Field: field, Old: old, New: new})
		}
	}
	cents := func(n int64) string { return strconv.FormatInt(n, 10) }

	add(rules.PriceChange, "cost_cents", cents(prev.CostCents), cents(cur.CostCents))
	add(rules.PriceChange, "rrp_cents", cents(prev.RRPCents), cents(cur.RRPCents))
	add(rules.AvailabilityChange, "availability.status", string(prev.Availability.Status), string(cur.Availability.Status))
	add(rules.DescriptionChange, "name", prev.Name, cur.Name)
	add(rules.DescriptionChange, "description", prev.Description, cur.Description)
	add(rules.ImagesChange, "image_urls", strings.Join(prev.ImageURLs, "\n"), strings.Join(cur.ImageURLs, "\n"))

	add(rules.AttributeChange, "ean", prev.EAN, cur.EAN)
	add(rules.AttributeChange, "mpn", prev.MPN, cur.MPN)
	add(rules.AttributeChange, "brand", prev.Brand, cur.Brand)
	add(rules.AttributeChange, "category", prev.Category, cur.Category)
	add(rules.AttributeChange, "color", prev.Color, cur.Color)
	add(rules.AttributeChange, "frame_size", prev.FrameSize, cur.FrameSize)
	add(rules.AttributeChange, "model_year", strconv.Itoa(prev.ModelYear), strconv.Itoa(cur.ModelYear))

	keys := map[string]bool{}
	for k := range prev.Attributes {
		keys[k] = true
	}
	for k := range cur.Attributes {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		add(rules.AttributeChange, "attributes."+k, prev.Attributes[k], cur.Attributes[k])
	}
	return out
}

// ProposeRemoved returns a deactivation proposal for every article that was
// in the previous import but is missing from the current one. Callers must
// check connector.SuspiciousDrop first.
func ProposeRemoved(previousSKUs, currentSKUs []string) []Proposal {
	cur := make(map[string]bool, len(currentSKUs))
	for _, s := range currentSKUs {
		cur[s] = true
	}
	var out []Proposal
	for _, s := range previousSKUs {
		if !cur[s] {
			out = append(out, Proposal{Type: rules.Deactivation, SKU: s})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SKU < out[j].SKU })
	return out
}
