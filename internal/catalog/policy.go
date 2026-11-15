package catalog

import (
	"context"
	"strings"

	"velopim/internal/db"
)

// Field groups a usage policy can be set for.
const (
	GroupImages     = "images"
	GroupTexts      = "texts"
	GroupPrices     = "prices"
	GroupAttributes = "attributes"
)

// What may be done with a group of supplier data.
const (
	// PolicyUse copies the data into the own catalog and from there to Shopify.
	PolicyUse = "use"
	// PolicyInternalOnly keeps the data visible in the supplier view but
	// never copies it into products.
	PolicyInternalOnly = "internal_only"
	// PolicyIgnore additionally creates no actions or tasks for changes.
	PolicyIgnore = "ignore"
)

// UsagePolicy says what may be done with one field group of one source.
// An empty brand applies to all brands of the source.
type UsagePolicy struct {
	SourceID string `json:"source_id"`
	Brand    string `json:"brand"`
	Group    string `json:"field_group"`
	Policy   string `json:"policy"`
}

// UsagePolicies is the configured set. Without a matching entry data may be
// used.
type UsagePolicies []UsagePolicy

// For returns the policy for a field group of a source's brand. An entry for
// the brand wins over the entry for all brands.
func (ps UsagePolicies) For(sourceID, brand, group string) string {
	result, specific := PolicyUse, false
	for _, p := range ps {
		if p.SourceID != sourceID || p.Group != group {
			continue
		}
		switch {
		case p.Brand != "" && strings.EqualFold(p.Brand, brand):
			result, specific = p.Policy, true
		case p.Brand == "" && !specific:
			result = p.Policy
		}
	}
	return result
}

// GroupOfField returns the field group a supplier item field belongs to.
func GroupOfField(field string) string {
	switch field {
	case "image_urls":
		return GroupImages
	case "name", "description":
		return GroupTexts
	case "cost_cents", "rrp_cents":
		return GroupPrices
	}
	return GroupAttributes
}

// ValidPolicy reports whether group and policy are known values.
func ValidPolicy(group, policy string) bool {
	switch group {
	case GroupImages, GroupTexts, GroupPrices, GroupAttributes:
	default:
		return false
	}
	switch policy {
	case PolicyUse, PolicyInternalOnly, PolicyIgnore:
		return true
	}
	return false
}

// LoadUsagePolicies reads the configured policies.
func LoadUsagePolicies(ctx context.Context, q *db.Queries) (UsagePolicies, error) {
	rows, err := q.ListUsagePolicies(ctx)
	if err != nil {
		return nil, err
	}
	out := make(UsagePolicies, 0, len(rows))
	for _, r := range rows {
		out = append(out, UsagePolicy{SourceID: r.SourceID, Brand: r.Brand, Group: r.FieldGroup, Policy: r.Policy})
	}
	return out, nil
}
