package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"velopim/internal/catalog"
	"velopim/internal/db"
)

// ProductDetail is the response of GET /api/products/{id}: the product, the
// other products cut from the same model, their history and internal notes.
type ProductDetail struct {
	Products []Product     `json:"products"`
	History  []FieldChange `json:"history"`
	Notes    []Note        `json:"notes"`
}

// FieldChange is one entry of the provenance log.
type FieldChange struct {
	ID         int64     `json:"id"`
	ProductID  int64     `json:"product_id"`
	VariantSKU *string   `json:"variant_sku"` // null for a product field
	Field      string    `json:"field"`
	Old        *string   `json:"old"`
	New        *string   `json:"new"`
	Origin     string    `json:"origin"` // import, rule, user, system
	Actor      *string   `json:"actor"`
	Rule       *string   `json:"rule"`
	Source     *string   `json:"source"` // supplier the change came from
	Reason     *string   `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}

// Note is an internal note. Notes never reach Shopify.
type Note struct {
	ID        int64     `json:"id"`
	ProductID int64     `json:"product_id"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func (s *server) getProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	detail, err := loadProductDetail(r.Context(), db.New(s.pool), id)
	if err != nil {
		internalError(w, err)
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func loadProductDetail(ctx context.Context, q *db.Queries, id int64) (*ProductDetail, error) {
	rows, err := q.ListProductsOfSameModel(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	plain := make([]db.ListProductsRow, len(rows))
	ids := make([]int64, len(rows))
	textIDs := make([]string, len(rows))
	for i, row := range rows {
		plain[i] = db.ListProductsRow(row)
		ids[i] = row.ID
		textIDs[i] = strconv.FormatInt(row.ID, 10)
	}
	detail := &ProductDetail{History: []FieldChange{}, Notes: []Note{}}
	if detail.Products, err = buildProducts(ctx, q, plain); err != nil {
		return nil, err
	}
	changes, err := q.ListFieldChangesOfProducts(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, c := range changes {
		detail.History = append(detail.History, FieldChange{ID: c.ID, ProductID: c.ProductID, VariantSKU: c.VariantSku,
			Field: c.Field, Old: c.OldValue, New: c.NewValue, Origin: c.Origin, Actor: c.Actor, Rule: c.RuleName,
			Source: c.SourceName, Reason: c.Reason, CreatedAt: c.CreatedAt})
	}
	notes, err := q.ListProductNotes(ctx, textIDs)
	if err != nil {
		return nil, err
	}
	for _, n := range notes {
		productID, _ := strconv.ParseInt(n.EntityID, 10, 64)
		detail.Notes = append(detail.Notes, Note{ID: n.ID, ProductID: productID, Body: n.Body, Author: n.Author, CreatedAt: n.CreatedAt})
	}
	return detail, nil
}

// patchProduct changes name, description or the store selection. Every
// change is recorded with the person who made it.
func (s *server) patchProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Name        *string `json:"name"`
		Description *string `json:"description_html"`
		Wanted      *bool   `json:"wanted"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		writeError(w, http.StatusUnprocessableEntity, "name must not be empty")
		return
	}
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
	q := db.New(tx)

	cur, err := q.GetProductText(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	record := func(field, oldValue, newValue string) error {
		if oldValue == newValue {
			return nil
		}
		return q.InsertCatalogFieldChange(ctx, db.InsertCatalogFieldChangeParams{EntityType: "product", EntityID: id,
			Field: field, OldValue: &oldValue, NewValue: &newValue, Origin: "user", Actor: &s.actor})
	}
	name, description := cur.Name, cur.DescriptionHtml
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		description = *in.Description
	}
	if name != cur.Name || description != cur.DescriptionHtml {
		if err := q.SetProductText(ctx, db.SetProductTextParams{ID: id, Name: name, DescriptionHtml: description}); err != nil {
			internalError(w, err)
			return
		}
		if err := errors.Join(record("name", cur.Name, name), record("description_html", cur.DescriptionHtml, description)); err != nil {
			internalError(w, err)
			return
		}
	}
	if in.Wanted != nil {
		was, err := q.GetListingWanted(ctx, id)
		if err != nil {
			internalError(w, err)
			return
		}
		if *in.Wanted && !was {
			// A variant without price cannot be sold.
			unpriced, err := q.CountUnpricedVariants(ctx, id)
			if err != nil {
				internalError(w, err)
				return
			}
			if unpriced > 0 {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "not possible as asked",
					"issues": []string{"Nicht alle Varianten haben einen Verkaufspreis"}})
				return
			}
		}
		if err := q.SetListingWanted(ctx, db.SetListingWantedParams{ProductID: id, Wanted: *in.Wanted}); err != nil {
			internalError(w, err)
			return
		}
		if err := record("in_store", strconv.FormatBool(was), strconv.FormatBool(*in.Wanted)); err != nil {
			internalError(w, err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) addProductNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		writeError(w, http.StatusUnprocessableEntity, "note must not be empty")
		return
	}
	q := db.New(s.pool)
	if _, err := q.GetProductText(r.Context(), id); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	if _, err := q.InsertProductNote(r.Context(), db.InsertProductNoteParams{
		EntityID: strconv.FormatInt(id, 10), Body: body, Author: s.actor}); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// patchVariant sets a variant's sales price.
func (s *server) patchVariant(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		PriceCents *int64 `json:"price_cents"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.PriceCents == nil {
		writeError(w, http.StatusUnprocessableEntity, "price_cents is required")
		return
	}
	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
	q := db.New(tx)
	if _, err := q.GetVariantProduct(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	err = catalog.SetVariantPrice(ctx, q, id, *in.PriceCents, nil, s.actor, "Preis von Hand geändert")
	var blocked *catalog.BlockedError
	if errors.As(err, &blocked) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "not possible as asked", "issues": blocked.Issues})
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
