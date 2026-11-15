// Package catalog builds the own catalog (products and variants) from
// supplier items.
package catalog

import (
	"fmt"
	"sort"
	"strings"

	"velopim/internal/connector"
)

// Axis is a characteristic supplier items of one model can differ in:
// "color", "frame_size" or "attributes.<key>" for a supplier attribute.
type Axis string

const (
	AxisColor     Axis = "color"
	AxisFrameSize Axis = "frame_size"

	attributePrefix = "attributes."
)

// MaxOptions is how many options a Shopify product can have.
const MaxOptions = 3

// Label is the option name shown to customers.
func (a Axis) Label() string {
	switch a {
	case AxisColor:
		return "Farbe"
	case AxisFrameSize:
		return "Rahmenhöhe"
	}
	return strings.TrimPrefix(string(a), attributePrefix)
}

func (a Axis) value(it connector.SupplierItem) string {
	switch a {
	case AxisColor:
		return strings.TrimSpace(it.Color)
	case AxisFrameSize:
		return strings.TrimSpace(it.FrameSize)
	}
	if key, ok := strings.CutPrefix(string(a), attributePrefix); ok {
		return strings.TrimSpace(it.Attributes[key])
	}
	return ""
}

// Config says how the items of one model are cut into products.
type Config struct {
	// SplitBy lists the axes that separate products: every distinct
	// combination of their values becomes a product of its own.
	SplitBy []Axis `json:"split_by"`
	// Options lists, in order, the axes that become product options. Axes
	// that are also in SplitBy or have no value on any item are left out.
	Options []Axis `json:"options"`
}

// DefaultConfig is one product per model with color and frame size as options.
var DefaultConfig = Config{Options: []Axis{AxisColor, AxisFrameSize}}

// ModelKey identifies a model: items with the same brand, name and model
// year belong together.
type ModelKey struct {
	Brand     string
	Name      string
	ModelYear int
}

// ModelOf returns the model a supplier item belongs to.
func ModelOf(it connector.SupplierItem) ModelKey {
	return ModelKey{Brand: strings.TrimSpace(it.Brand), Name: strings.TrimSpace(it.Name), ModelYear: it.ModelYear}
}

// Title is the model as shown to a person, e.g. "Beispielrad Trekking 5.4 (2027)".
func (k ModelKey) Title() string {
	t := strings.TrimSpace(k.Brand + " " + k.Name)
	if k.ModelYear != 0 {
		t += fmt.Sprintf(" (%d)", k.ModelYear)
	}
	return t
}

// Settings hold the default cut and the exceptions for single models.
type Settings struct {
	Default  Config
	PerModel map[ModelKey]Config
}

func (s Settings) configFor(k ModelKey) Config {
	if c, ok := s.PerModel[k]; ok {
		return c
	}
	return s.Default
}

// AxisValue is one axis with the value a product was split off for.
type AxisValue struct {
	Axis  Axis
	Value string
}

// Product is one proposed product with its variants.
type Product struct {
	Model ModelKey
	// Split holds the values this product was separated by; empty when the
	// model is one product.
	Split []AxisValue
	// Title is the model name followed by the split values.
	Title       string
	OptionAxes  []Axis
	OptionNames []string
	Variants    []Variant
	// Issues lists what a person has to fix before this can go to Shopify.
	Issues []string
}

// Variant is one supplier item as a variant of a product.
type Variant struct {
	SKU          string
	OptionValues []string // parallel to Product.OptionAxes
}

// Group cuts supplier items into products. The result is deterministic:
// products are sorted by model and split values, variants by option values.
func Group(items []connector.SupplierItem, s Settings) []Product {
	byModel := map[ModelKey][]connector.SupplierItem{}
	for _, it := range items {
		k := ModelOf(it)
		byModel[k] = append(byModel[k], it)
	}
	var out []Product
	for model, modelItems := range byModel {
		out = append(out, groupModel(model, modelItems, s.configFor(model))...)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Model != b.Model {
			if a.Model.Brand != b.Model.Brand {
				return a.Model.Brand < b.Model.Brand
			}
			if a.Model.Name != b.Model.Name {
				return a.Model.Name < b.Model.Name
			}
			return a.Model.ModelYear < b.Model.ModelYear
		}
		return splitKey(a.Split) < splitKey(b.Split)
	})
	return out
}

func groupModel(model ModelKey, items []connector.SupplierItem, cfg Config) []Product {
	bySplit := map[string][]connector.SupplierItem{}
	splits := map[string][]AxisValue{}
	for _, it := range items {
		split := make([]AxisValue, len(cfg.SplitBy))
		for i, axis := range cfg.SplitBy {
			split[i] = AxisValue{Axis: axis, Value: axis.value(it)}
		}
		key := splitKey(split)
		bySplit[key] = append(bySplit[key], it)
		splits[key] = split
	}

	isSplit := map[Axis]bool{}
	for _, axis := range cfg.SplitBy {
		isSplit[axis] = true
	}

	var out []Product
	for key, group := range bySplit {
		p := Product{Model: model, Split: splits[key], Title: model.Name}
		for _, sv := range p.Split {
			if sv.Value == "" {
				p.Issues = append(p.Issues, fmt.Sprintf("Artikel ohne Wert für das Trennmerkmal %q", sv.Axis.Label()))
				continue
			}
			p.Title += " " + sv.Value
		}
		for _, axis := range cfg.Options {
			if isSplit[axis] || !anyValue(group, axis) {
				continue
			}
			p.OptionAxes = append(p.OptionAxes, axis)
			p.OptionNames = append(p.OptionNames, axis.Label())
		}
		if len(p.OptionAxes) > MaxOptions {
			p.Issues = append(p.Issues, fmt.Sprintf("%d Optionen, Shopify erlaubt %d", len(p.OptionAxes), MaxOptions))
		}

		seen := map[string]string{}
		for _, it := range group {
			v := Variant{SKU: it.SKU, OptionValues: make([]string, len(p.OptionAxes))}
			for i, axis := range p.OptionAxes {
				v.OptionValues[i] = axis.value(it)
				if v.OptionValues[i] == "" {
					p.Issues = append(p.Issues, fmt.Sprintf("Artikel %s hat keinen Wert für %q", it.SKU, axis.Label()))
				}
			}
			p.Variants = append(p.Variants, v)
		}
		sort.Slice(p.Variants, func(i, j int) bool {
			a, b := p.Variants[i], p.Variants[j]
			if ka, kb := strings.Join(a.OptionValues, "\x00"), strings.Join(b.OptionValues, "\x00"); ka != kb {
				return ka < kb
			}
			return a.SKU < b.SKU
		})
		for _, v := range p.Variants {
			combo := strings.Join(v.OptionValues, "\x00")
			if other, dup := seen[combo]; dup {
				p.Issues = append(p.Issues, fmt.Sprintf("Artikel %s und %s sind nicht unterscheidbar (%s)",
					other, v.SKU, describe(p.OptionNames, v.OptionValues)))
				continue
			}
			seen[combo] = v.SKU
		}
		sort.Strings(p.Issues)
		out = append(out, p)
	}
	return out
}

func anyValue(items []connector.SupplierItem, axis Axis) bool {
	for _, it := range items {
		if axis.value(it) != "" {
			return true
		}
	}
	return false
}

func splitKey(split []AxisValue) string {
	parts := make([]string, len(split))
	for i, sv := range split {
		parts[i] = sv.Value
	}
	return strings.Join(parts, "\x00")
}

func describe(names, values []string) string {
	if len(names) == 0 {
		return "keine Option unterscheidet sie"
	}
	parts := make([]string, len(names))
	for i := range names {
		parts[i] = names[i] + ": " + values[i]
	}
	return strings.Join(parts, ", ")
}
