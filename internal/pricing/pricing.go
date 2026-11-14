// Package pricing derives the gross sales price from the net purchase price.
// It is pure; all amounts are integer cents.
package pricing

import (
	"errors"
	"math"
)

// Mode selects how the net sales price is derived from the purchase price.
type Mode string

const (
	// Markup multiplies the purchase price by Value (e.g. 1.6).
	Markup Mode = "markup"
	// TargetMargin sets the price so that Value percent of the net sales
	// price is margin (e.g. 35).
	TargetMargin Mode = "target_margin"
)

// Rounding selects the shape of the final gross price.
type Rounding string

const (
	RoundNone  Rounding = "none"       // nearest cent
	RoundEuro  Rounding = "whole_euro" // 1234.00
	RoundEnd99 Rounding = "end_99"     // 1233.99
	RoundEnd9  Rounding = "end_9"      // 1229.00, 1239.00
)

// Config is one calculation rule (later stored per brand or category).
type Config struct {
	Mode     Mode     `json:"mode"`
	Value    float64  `json:"value"`
	VATPct   float64  `json:"vat_pct"`
	Rounding Rounding `json:"rounding"`
	// CapAtRRP keeps the price at or below the supplier's RRP when one is known.
	CapAtRRP bool `json:"cap_at_rrp"`
}

// Result is the outcome of Calculate.
type Result struct {
	PriceCents int64 // gross sales price
	// CompareAtCents is the RRP when the price is below it, otherwise 0.
	CompareAtCents int64
	// MarginPct is the margin on the net sales price in percent.
	MarginPct float64
	// Capped reports that the calculated price was lowered to the RRP.
	Capped bool
}

// Calculate derives the sales price. costCents is net, rrpCents is gross and
// may be 0 when unknown.
func Calculate(costCents, rrpCents int64, c Config) (Result, error) {
	if costCents <= 0 {
		return Result{}, errors.New("pricing: purchase price must be positive")
	}
	var net float64
	switch c.Mode {
	case Markup:
		if c.Value <= 0 {
			return Result{}, errors.New("pricing: markup factor must be positive")
		}
		net = float64(costCents) * c.Value
	case TargetMargin:
		if c.Value < 0 || c.Value >= 100 {
			return Result{}, errors.New("pricing: target margin must be in [0, 100)")
		}
		net = float64(costCents) / (1 - c.Value/100)
	default:
		return Result{}, errors.New("pricing: unknown mode " + string(c.Mode))
	}
	if c.VATPct < 0 {
		return Result{}, errors.New("pricing: VAT must not be negative")
	}

	price, err := round(net*(1+c.VATPct/100), c.Rounding)
	if err != nil {
		return Result{}, err
	}
	res := Result{PriceCents: price}
	if c.CapAtRRP && rrpCents > 0 && price > rrpCents {
		res.PriceCents = rrpCents
		res.Capped = true
	}
	if rrpCents > res.PriceCents {
		res.CompareAtCents = rrpCents
	}
	res.MarginPct = MarginPct(costCents, res.PriceCents, c.VATPct)
	return res, nil
}

// MarginPct returns the margin of a gross price over a net purchase price, in
// percent of the net sales price. It is negative when selling below cost.
func MarginPct(costCents, grossCents int64, vatPct float64) float64 {
	net := float64(grossCents) / (1 + vatPct/100)
	if net <= 0 {
		return math.Inf(-1)
	}
	return (net - float64(costCents)) / net * 100
}

// ChangePct returns the signed change from old to new in percent.
func ChangePct(oldCents, newCents int64) float64 {
	if oldCents == 0 {
		return math.Inf(1)
	}
	return float64(newCents-oldCents) / float64(oldCents) * 100
}

func round(cents float64, r Rounding) (int64, error) {
	switch r {
	case RoundNone, "":
		return int64(math.Round(cents)), nil
	case RoundEuro:
		return int64(math.Round(cents/100)) * 100, nil
	case RoundEnd99:
		// Nearest amount of the form N.99, never below 0.99.
		n := math.Max(0, math.Round((cents-99)/100))
		return int64(n)*100 + 99, nil
	case RoundEnd9:
		// Nearest whole-euro amount ending in 9, never below 9.00.
		k := math.Max(0, math.Round((cents/100-9)/10))
		return (int64(k)*10 + 9) * 100, nil
	}
	return 0, errors.New("pricing: unknown rounding " + string(r))
}
