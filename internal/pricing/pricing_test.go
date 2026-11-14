package pricing

import (
	"math"
	"testing"
)

func TestCalculate(t *testing.T) {
	tests := []struct {
		name          string
		cost, rrp     int64
		cfg           Config
		wantPrice     int64
		wantCompareAt int64
		wantCapped    bool
	}{
		{"markup, no rounding", 100000, 0,
			Config{Mode: Markup, Value: 1.5, VATPct: 19}, 178500, 0, false},
		{"target margin 35 percent", 65000, 0,
			Config{Mode: TargetMargin, Value: 35, VATPct: 19}, 119000, 0, false},
		{"round to whole euro", 100000, 0,
			Config{Mode: Markup, Value: 1.513, VATPct: 19, Rounding: RoundEuro}, 180000, 0, false},
		{"round to .99", 100000, 0,
			Config{Mode: Markup, Value: 1.5, VATPct: 19, Rounding: RoundEnd99}, 178499, 0, false},
		{"round to .99 goes up when closer", 100000, 0,
			Config{Mode: Markup, Value: 1.5008, VATPct: 19, Rounding: RoundEnd99}, 178599, 0, false},
		{"round to euro ending in 9", 100000, 0,
			Config{Mode: Markup, Value: 1.5, VATPct: 19, Rounding: RoundEnd9}, 178900, 0, false},
		{"round to euro ending in 9, downwards", 100000, 0,
			Config{Mode: Markup, Value: 1.49, VATPct: 19, Rounding: RoundEnd9}, 176900, 0, false},
		{"capped at RRP", 100000, 169900,
			Config{Mode: Markup, Value: 1.5, VATPct: 19, CapAtRRP: true}, 169900, 0, true},
		{"above RRP without cap", 100000, 169900,
			Config{Mode: Markup, Value: 1.5, VATPct: 19}, 178500, 0, false},
		{"below RRP shows compare-at price", 100000, 199900,
			Config{Mode: Markup, Value: 1.5, VATPct: 19, CapAtRRP: true}, 178500, 199900, false},
		{"tiny amounts never round to zero", 20, 0,
			Config{Mode: Markup, Value: 1.2, VATPct: 19, Rounding: RoundEnd99}, 99, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Calculate(tt.cost, tt.rrp, tt.cfg)
			if err != nil {
				t.Fatal(err)
			}
			if got.PriceCents != tt.wantPrice || got.CompareAtCents != tt.wantCompareAt || got.Capped != tt.wantCapped {
				t.Errorf("got %+v, want price=%d compareAt=%d capped=%v", got, tt.wantPrice, tt.wantCompareAt, tt.wantCapped)
			}
		})
	}
}

func TestCalculateErrors(t *testing.T) {
	bad := []struct {
		name string
		cost int64
		cfg  Config
	}{
		{"zero cost", 0, Config{Mode: Markup, Value: 1.5}},
		{"zero markup", 100, Config{Mode: Markup}},
		{"margin of 100 percent", 100, Config{Mode: TargetMargin, Value: 100}},
		{"unknown mode", 100, Config{Mode: "x", Value: 1}},
		{"unknown rounding", 100, Config{Mode: Markup, Value: 1.5, Rounding: "x"}},
		{"negative VAT", 100, Config{Mode: Markup, Value: 1.5, VATPct: -1}},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Calculate(tt.cost, 0, tt.cfg); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestMarginPct(t *testing.T) {
	// 1190.00 gross is 1000.00 net; cost 650.00 leaves 35 percent.
	if got := MarginPct(65000, 119000, 19); math.Abs(got-35) > 1e-9 {
		t.Errorf("margin = %v, want 35", got)
	}
	if got := MarginPct(120000, 119000, 19); got >= 0 {
		t.Errorf("selling below cost must give a negative margin, got %v", got)
	}
	res, err := Calculate(65000, 0, Config{Mode: TargetMargin, Value: 35, VATPct: 19})
	if err != nil || math.Abs(res.MarginPct-35) > 0.01 {
		t.Errorf("result margin = %v (err %v), want about 35", res.MarginPct, err)
	}
}

func TestChangePct(t *testing.T) {
	if got := ChangePct(200000, 190000); got != -5 {
		t.Errorf("got %v, want -5", got)
	}
	if got := ChangePct(0, 100); !math.IsInf(got, 1) {
		t.Errorf("got %v, want +Inf", got)
	}
}
