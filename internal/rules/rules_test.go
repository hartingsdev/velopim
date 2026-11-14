package rules

import "testing"

// Rule set modelled on the examples from the project brief.
var testRules = []Rule{
	{ID: 1, Name: "new products need approval", Enabled: true, ActionType: NewProduct, Outcome: Approval},
	{ID: 2, Name: "availability is automatic", Enabled: true, ActionType: AvailabilityChange, Outcome: Auto},
	{ID: 3, Name: "small price change with healthy margin", Enabled: true, ActionType: PriceChange, Outcome: Auto,
		Conditions: []Condition{{PriceChangeAbsPct, LT, 5}, {MarginPct, GT, 20}}},
	{ID: 4, Name: "margin below minimum", Enabled: true, ActionType: PriceChange, Outcome: Approval, Guard: true,
		Conditions: []Condition{{MarginPct, LT, 10}}},
	{ID: 5, Name: "brand Victoria: any price change", Enabled: true, ActionType: PriceChange, Outcome: Auto,
		Scope: Scope{Brand: "Victoria"}},
	{ID: 6, Name: "Victoria e-bikes: approval", Enabled: true, ActionType: PriceChange, Outcome: Approval,
		Scope: Scope{Brand: "Victoria", Category: "e-bike"}},
	{ID: 7, Name: "disabled auto for new products", Enabled: false, ActionType: NewProduct, Outcome: Auto,
		Scope: Scope{Brand: "Conway"}},
	{ID: 8, Name: "Conway from hartje: auto descriptions", Enabled: true, ActionType: DescriptionChange, Outcome: Auto,
		Scope: Scope{SourceID: "hartje", Brand: "Conway"}},
	{ID: 9, Name: "Conway city bikes: approve descriptions", Enabled: true, ActionType: DescriptionChange, Outcome: Approval,
		Scope: Scope{Brand: "Conway", Category: "city"}},
}

func facts(changePct, margin float64) map[Fact]float64 {
	abs := changePct
	if abs < 0 {
		abs = -abs
	}
	return map[Fact]float64{PriceChangePct: changePct, PriceChangeAbsPct: abs, MarginPct: margin}
}

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name       string
		action     Action
		wantOut    Outcome
		wantRule   int64
		wantReason string
	}{
		{"new product always needs approval",
			Action{Type: NewProduct, Brand: "Contoura"}, Approval, 1, ReasonSpecific},
		{"disabled rule is ignored",
			Action{Type: NewProduct, Brand: "Conway"}, Approval, 1, ReasonSpecific},
		{"availability change is automatic",
			Action{Type: AvailabilityChange, Brand: "Contoura"}, Auto, 2, ReasonSpecific},
		{"small price change with good margin is automatic",
			Action{Type: PriceChange, Brand: "Contoura", Facts: facts(3, 25)}, Auto, 3, ReasonSpecific},
		{"price change at the threshold is not automatic",
			Action{Type: PriceChange, Brand: "Contoura", Facts: facts(5, 25)}, Approval, 0, ReasonNoRule},
		{"large price change falls back to approval",
			Action{Type: PriceChange, Brand: "Contoura", Facts: facts(-12, 25)}, Approval, 0, ReasonNoRule},
		{"good change but thin margin falls back to approval",
			Action{Type: PriceChange, Brand: "Contoura", Facts: facts(2, 15)}, Approval, 0, ReasonNoRule},
		{"margin below minimum hits the guard",
			Action{Type: PriceChange, Brand: "Contoura", Facts: facts(-3, 8)}, Approval, 4, ReasonGuard},
		{"brand rule overrides the global rule",
			Action{Type: PriceChange, Brand: "Victoria", Category: "trekking", Facts: facts(15, 25)}, Auto, 5, ReasonSpecific},
		{"guard beats a more specific auto rule",
			Action{Type: PriceChange, Brand: "Victoria", Category: "trekking", Facts: facts(-30, 5)}, Approval, 4, ReasonGuard},
		{"brand+category rule beats the brand rule",
			Action{Type: PriceChange, Brand: "Victoria", Category: "e-bike", Facts: facts(1, 30)}, Approval, 6, ReasonSpecific},
		{"missing facts never satisfy a condition",
			Action{Type: PriceChange, Brand: "Contoura"}, Approval, 0, ReasonNoRule},
		{"equally specific rules disagree: approval wins",
			Action{Type: DescriptionChange, SourceID: "hartje", Brand: "Conway", Category: "city"}, Approval, 9, ReasonConflict},
		{"scope mismatch does not match",
			Action{Type: DescriptionChange, SourceID: "other", Brand: "Conway", Category: "mtb"}, Approval, 0, ReasonNoRule},
		{"no rule for the action type",
			Action{Type: Deactivation, Brand: "Conway"}, Approval, 0, ReasonNoRule},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(testRules, tt.action)
			if got.Outcome != tt.wantOut || got.RuleID != tt.wantRule || got.Reason != tt.wantReason {
				t.Errorf("got %+v, want outcome=%s rule=%d reason=%q", got, tt.wantOut, tt.wantRule, tt.wantReason)
			}
		})
	}
}

func TestEvaluateNoRules(t *testing.T) {
	got := Evaluate(nil, Action{Type: AvailabilityChange})
	if got.Outcome != Approval || got.RuleID != 0 {
		t.Errorf("got %+v, want approval without rule", got)
	}
}

func TestEvaluateIsOrderIndependent(t *testing.T) {
	reversed := make([]Rule, len(testRules))
	for i, r := range testRules {
		reversed[len(testRules)-1-i] = r
	}
	a := Action{Type: DescriptionChange, SourceID: "hartje", Brand: "Conway", Category: "city"}
	if got, want := Evaluate(reversed, a), Evaluate(testRules, a); got != want {
		t.Errorf("order changed the decision: %+v vs %+v", got, want)
	}
}
