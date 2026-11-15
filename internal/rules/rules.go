// Package rules decides whether an action is applied automatically or needs
// human approval. It is pure: no database, no I/O. Callers compute the facts
// (price change, margin, ...) and pass them in.
package rules

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ActionType is the kind of change an action proposes.
type ActionType string

const (
	NewProduct         ActionType = "new_product"
	PriceChange        ActionType = "price_change"
	AvailabilityChange ActionType = "availability_status_change"
	DescriptionChange  ActionType = "description_change"
	ImagesChange       ActionType = "images_change"
	Deactivation       ActionType = "deactivation"
	AttributeChange    ActionType = "attribute_change"
)

// Outcome is what happens to an action.
type Outcome string

const (
	Auto     Outcome = "auto"
	Approval Outcome = "approval"
)

// Fact names a numeric value a condition can test. The catalog is fixed so
// rules stay editable through a form.
type Fact string

const (
	// PriceChangePct is the signed change of the sales price in percent
	// (negative = price drop).
	PriceChangePct Fact = "price_change_pct"
	// PriceChangeAbsPct is the absolute value of PriceChangePct.
	PriceChangeAbsPct Fact = "price_change_abs_pct"
	// MarginPct is the margin of the new net sales price over the purchase
	// price, in percent of the net sales price.
	MarginPct Fact = "margin_pct"
	// CostChangeCents is the signed change of the supplier's purchase price
	// in cents; negative when the supplier got cheaper.
	CostChangeCents Fact = "cost_change_cents"
)

// Op is a comparison operator.
type Op string

const (
	LT  Op = "lt"
	LTE Op = "lte"
	GT  Op = "gt"
	GTE Op = "gte"
)

// Condition compares one fact with a fixed value.
type Condition struct {
	Fact  Fact    `json:"fact"`
	Op    Op      `json:"op"`
	Value float64 `json:"value"`
}

// Scope limits a rule. Empty fields match anything.
type Scope struct {
	SourceID string `json:"source_id,omitempty"`
	Brand    string `json:"brand,omitempty"`
	Category string `json:"category,omitempty"`
}

// Rule maps matching actions to an outcome.
type Rule struct {
	ID         int64       `json:"id"`
	Name       string      `json:"name"`
	Enabled    bool        `json:"enabled"`
	ActionType ActionType  `json:"action_type"`
	Scope      Scope       `json:"scope"`
	Conditions []Condition `json:"conditions"`
	Outcome    Outcome     `json:"outcome"`
	// Guard marks an approval rule that always wins when it matches, no
	// matter how specific other matching rules are ("immer Freigabe").
	Guard bool `json:"guard"`
}

// Action is the input to Evaluate.
type Action struct {
	Type     ActionType
	SourceID string
	Brand    string
	Category string
	Facts    map[Fact]float64
}

// Decision is the result of Evaluate. RuleID is 0 when no rule matched.
type Decision struct {
	Outcome Outcome
	RuleID  int64
	Reason  string
}

// Reasons recorded with a decision.
const (
	ReasonGuard    = "guard rule matched"
	ReasonSpecific = "most specific rule matched"
	ReasonConflict = "equally specific rules disagree, approval wins"
	ReasonNoRule   = "no rule matched, approval by default"
)

// Evaluate picks the outcome for an action:
//  1. A matching guard rule always forces approval.
//  2. Otherwise the most specific matching rule wins (most scope fields set).
//  3. If equally specific rules disagree, approval wins.
//  4. If nothing matches, approval.
//
// A condition whose fact is missing does not match.
func Evaluate(rs []Rule, a Action) Decision {
	var matched []Rule
	for _, r := range rs {
		if r.Enabled && matches(r, a) {
			matched = append(matched, r)
		}
	}
	if len(matched) == 0 {
		return Decision{Outcome: Approval, Reason: ReasonNoRule}
	}
	// Deterministic order: most specific first, approval before auto, lowest ID first.
	sort.SliceStable(matched, func(i, j int) bool {
		si, sj := specificity(matched[i].Scope), specificity(matched[j].Scope)
		if si != sj {
			return si > sj
		}
		if matched[i].Outcome != matched[j].Outcome {
			return matched[i].Outcome == Approval
		}
		return matched[i].ID < matched[j].ID
	})

	for _, r := range matched {
		if r.Guard && r.Outcome == Approval {
			return Decision{Outcome: Approval, RuleID: r.ID, Reason: ReasonGuard}
		}
	}

	top := matched[0]
	reason := ReasonSpecific
	if top.Outcome == Approval {
		for _, r := range matched[1:] {
			if specificity(r.Scope) != specificity(top.Scope) {
				break
			}
			if r.Outcome == Auto {
				reason = ReasonConflict
				break
			}
		}
	}
	return Decision{Outcome: top.Outcome, RuleID: top.ID, Reason: reason}
}

func matches(r Rule, a Action) bool {
	if r.ActionType != a.Type {
		return false
	}
	s := r.Scope
	if s.SourceID != "" && s.SourceID != a.SourceID {
		return false
	}
	if s.Brand != "" && s.Brand != a.Brand {
		return false
	}
	if s.Category != "" && s.Category != a.Category {
		return false
	}
	for _, c := range r.Conditions {
		v, ok := a.Facts[c.Fact]
		if !ok || !compare(v, c.Op, c.Value) {
			return false
		}
	}
	return true
}

func compare(v float64, op Op, want float64) bool {
	switch op {
	case LT:
		return v < want
	case LTE:
		return v <= want
	case GT:
		return v > want
	case GTE:
		return v >= want
	}
	return false
}

func specificity(s Scope) int {
	n := 0
	if s.SourceID != "" {
		n++
	}
	if s.Brand != "" {
		n++
	}
	if s.Category != "" {
		n++
	}
	return n
}

// Validate reports the first reason a rule cannot be stored.
func (r Rule) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("rules: name is required")
	}
	switch r.ActionType {
	case NewProduct, PriceChange, AvailabilityChange, DescriptionChange, ImagesChange, Deactivation, AttributeChange:
	default:
		return fmt.Errorf("rules: unknown action type %q", r.ActionType)
	}
	switch r.Outcome {
	case Auto, Approval:
	default:
		return fmt.Errorf("rules: unknown outcome %q", r.Outcome)
	}
	if r.Guard && r.Outcome != Approval {
		return errors.New("rules: a guard rule can only require approval")
	}
	for _, c := range r.Conditions {
		switch c.Fact {
		case PriceChangePct, PriceChangeAbsPct, MarginPct, CostChangeCents:
		default:
			return fmt.Errorf("rules: unknown fact %q", c.Fact)
		}
		if r.ActionType != PriceChange {
			return fmt.Errorf("rules: conditions are only available for price changes, not for %q", r.ActionType)
		}
		switch c.Op {
		case LT, LTE, GT, GTE:
		default:
			return fmt.Errorf("rules: unknown operator %q", c.Op)
		}
	}
	return nil
}
