package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"velopim/internal/catalog"
	"velopim/internal/db"
	"velopim/internal/describe"
)

// Describer drafts product descriptions; nil when no text model is configured.
type Describer interface {
	Generate(ctx context.Context, in describe.Input) (describe.Draft, error)
}

// WithDescriber switches on description drafts.
func WithDescriber(d Describer) Option { return func(s *server) { s.describer = d } }

// descriptionFacts collects what may be told to the text model about a
// product: the product's own fields, the choices a customer has, and the
// supplier data the usage policy allows (docs/decisions/0032). The
// supplier's wording is only used where texts may be used in the shop; its
// technical data unless the policy says to ignore it. Prices, supplier
// names and notes are never part of it.
func descriptionFacts(ctx context.Context, q *db.Queries, productID int64) (describe.Input, error) {
	p, err := q.GetProductFacts(ctx, productID)
	if err != nil {
		return describe.Input{}, err
	}
	in := describe.Input{Brand: p.Brand, Name: p.Name, Category: p.Category, Attributes: map[string][]string{}}
	if p.ModelYear != nil {
		in.ModelYear = int(*p.ModelYear)
	}
	var optionNames []string
	if err := json.Unmarshal(p.OptionNames, &optionNames); err != nil {
		return describe.Input{}, fmt.Errorf("option names of product %d: %w", productID, err)
	}
	in.Options = make([]describe.Option, len(optionNames))
	for i, name := range optionNames {
		in.Options[i].Name = name
	}
	policies, err := catalog.LoadUsagePolicies(ctx, q)
	if err != nil {
		return describe.Input{}, err
	}
	rows, err := q.ListProductVariantFacts(ctx, productID)
	if err != nil {
		return describe.Input{}, err
	}
	add := func(list []string, value string) []string {
		value = strings.TrimSpace(value)
		for _, have := range list {
			if have == value {
				return list
			}
		}
		if value == "" {
			return list
		}
		return append(list, value)
	}
	for _, row := range rows {
		var values []string
		_ = json.Unmarshal(row.OptionValues, &values)
		for i := range in.Options {
			if i < len(values) {
				in.Options[i].Values = add(in.Options[i].Values, values[i])
			}
		}
		if row.SourceID == nil {
			continue
		}
		brand := ""
		if row.SupplierBrand != nil {
			brand = *row.SupplierBrand
		}
		if policies.For(*row.SourceID, brand, catalog.GroupAttributes) != catalog.PolicyIgnore {
			var attrs map[string]any
			_ = json.Unmarshal(row.Attributes, &attrs)
			for k, v := range attrs {
				if s, ok := v.(string); ok {
					in.Attributes[k] = add(in.Attributes[k], s)
				} else if v != nil {
					in.Attributes[k] = add(in.Attributes[k], fmt.Sprint(v))
				}
			}
		}
		// The first supplier text is the material; variants of one model
		// usually share it.
		if in.SupplierText == "" && row.SupplierDescription != nil &&
			policies.For(*row.SourceID, brand, catalog.GroupTexts) == catalog.PolicyUse {
			in.SupplierText = strings.TrimSpace(*row.SupplierDescription)
		}
	}
	return in, nil
}

// draftDescription suggests a description for a product. It stores nothing:
// the draft goes into the form, and only saving it there changes the product.
func (s *server) draftDescription(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct{}
	if !readJSON(w, r, &in) {
		return
	}
	if s.describer == nil {
		writeError(w, http.StatusConflict, "Textvorschläge sind nicht eingerichtet (ANTHROPIC_API_KEY fehlt)")
		return
	}
	facts, err := descriptionFacts(r.Context(), db.New(s.pool), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	draft, err := s.describer.Generate(r.Context(), facts)
	if err != nil {
		log.Printf("description draft for product %d: %v", id, err)
		message := "Der Textvorschlag konnte nicht erstellt werden: " + err.Error()
		if errors.Is(err, describe.ErrRefused) {
			message = "Das Textmodell hat den Vorschlag abgelehnt."
		}
		writeError(w, http.StatusBadGateway, message)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"description_html": draft.HTML()})
}
