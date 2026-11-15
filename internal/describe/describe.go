// Package describe drafts product descriptions from the facts velopim holds
// about a product. A draft is a suggestion: nothing is stored until a person
// has read it and saved it (docs/decisions/0032).
package describe

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"sort"
	"strings"
)

// Input is everything that is sent to the text model. It holds no purchase
// prices, no supplier names and no internal notes, and must stay that way.
type Input struct {
	Brand     string
	Name      string
	ModelYear int // 0 when unknown
	Category  string
	// Options are the choices a customer has, e.g. "Rahmenhöhe": 50, 54.
	Options []Option
	// Attributes are the supplier's technical data; an attribute whose
	// variants differ lists every value.
	Attributes map[string][]string
	// SupplierText is the supplier's own description, as source material.
	SupplierText string
}

// Option is one choice with its values.
type Option struct {
	Name   string
	Values []string
}

// Draft is a suggested description.
type Draft struct {
	Paragraphs []string `json:"paragraphs"`
	Highlights []string `json:"highlights"`
}

// Schema is the JSON schema the model's answer must follow.
var Schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"paragraphs": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"highlights": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
	"required":             []string{"paragraphs", "highlights"},
	"additionalProperties": false,
}

// System is the standing instruction for the text model.
const System = `Du schreibst Produktbeschreibungen für den Onlineshop eines deutschen Fahrradhändlers.

Der Text hilft Kundinnen und Kunden zu entscheiden, ob das Rad zu ihnen passt: für wen und wofür es gedacht ist, und was es technisch ausmacht. Schreibe sachlich und konkret, ohne Werbefloskeln und Superlative, und sprich die Leser nicht direkt an.

Verwende ausschließlich Tatsachen aus den Angaben in der Nachricht. Was dort nicht steht, weißt du nicht: keine Ausstattung, keine Maße, keine Gewichte und keine Eigenschaften ergänzen, auch wenn sie bei solchen Rädern üblich sind. Sind die Angaben dünn, wird der Text kurz. Der Text des Lieferanten ist nur Material: Übernimm seine Tatsachen, aber formuliere neu, statt Sätze zu kopieren.

Preise, Lieferzeiten und Verfügbarkeit gehören nicht in den Text; der Shop zeigt sie getrennt und sie ändern sich. Die wählbaren Farben und Größen zeigt der Shop ebenfalls selbst, zähle sie nicht auf.

Liefere zwei bis drei Absätze Fließtext und vier bis sechs Stichpunkte mit den wichtigsten technischen Merkmalen (weniger, wenn die Angaben nicht mehr hergeben). Reiner Text, kein HTML und kein Markdown.`

// Prompt renders the facts about a product as the message for the model.
// The same input always gives the same text.
func Prompt(in Input) string {
	var b strings.Builder
	line := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(&b, "%s: %s\n", label, strings.TrimSpace(value))
		}
	}
	b.WriteString("<produkt>\n")
	line("Marke", in.Brand)
	line("Modell", in.Name)
	if in.ModelYear > 0 {
		line("Modelljahr", fmt.Sprint(in.ModelYear))
	}
	line("Kategorie", in.Category)
	for _, o := range in.Options {
		line("Wählbar – "+o.Name, strings.Join(o.Values, ", "))
	}
	b.WriteString("</produkt>\n")

	keys := make([]string, 0, len(in.Attributes))
	for k, values := range in.Attributes {
		if len(values) > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		b.WriteString("<technische_daten>\n")
		for _, k := range keys {
			line(k, strings.Join(in.Attributes[k], " / "))
		}
		b.WriteString("</technische_daten>\n")
	}
	if text := strings.TrimSpace(in.SupplierText); text != "" {
		b.WriteString("<lieferantentext>\n" + text + "\n</lieferantentext>\n")
	}
	b.WriteString("Schreibe die Produktbeschreibung für dieses Rad.")
	return b.String()
}

// ErrEmptyDraft means the model answered without any text.
var ErrEmptyDraft = errors.New("describe: the draft is empty")

// Parse reads the model's answer. Empty entries are dropped.
func Parse(answer string) (Draft, error) {
	var raw Draft
	if err := json.Unmarshal([]byte(answer), &raw); err != nil {
		return Draft{}, fmt.Errorf("describe: answer is not the expected JSON: %w", err)
	}
	d := Draft{Paragraphs: clean(raw.Paragraphs), Highlights: clean(raw.Highlights)}
	if len(d.Paragraphs) == 0 {
		return Draft{}, ErrEmptyDraft
	}
	return d, nil
}

func clean(in []string) []string {
	out := []string{}
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// HTML renders the draft for the product's description. The model's text is
// escaped: whatever it wrote, only paragraphs and one list come out.
func (d Draft) HTML() string {
	var b strings.Builder
	for _, p := range d.Paragraphs {
		b.WriteString("<p>" + html.EscapeString(p) + "</p>\n")
	}
	if len(d.Highlights) > 0 {
		b.WriteString("<ul>\n")
		for _, h := range d.Highlights {
			b.WriteString("  <li>" + html.EscapeString(h) + "</li>\n")
		}
		b.WriteString("</ul>\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
