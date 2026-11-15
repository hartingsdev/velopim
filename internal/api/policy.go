package api

import (
	"net/http"

	"velopim/internal/catalog"
	"velopim/internal/db"
)

// UsagePolicyList is the response of GET /api/usage-policies.
type UsagePolicyList struct {
	Policies []catalog.UsagePolicy `json:"policies"`
	Sources  []Source              `json:"sources"`
}

func (s *server) listUsagePolicies(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.pool)
	policies, err := catalog.LoadUsagePolicies(r.Context(), q)
	if err != nil {
		internalError(w, err)
		return
	}
	sources, err := q.ListSources(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	out := UsagePolicyList{Policies: policies, Sources: []Source{}}
	for _, src := range sources {
		out.Sources = append(out.Sources, Source{ID: src.ID, Name: src.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

// putUsagePolicy sets what may be done with one field group of a source,
// for all brands or one brand. "use" for all brands is the default and is
// stored as no entry.
func (s *server) putUsagePolicy(w http.ResponseWriter, r *http.Request) {
	var in catalog.UsagePolicy
	if !readJSON(w, r, &in) {
		return
	}
	if in.SourceID == "" || !catalog.ValidPolicy(in.Group, in.Policy) {
		writeError(w, http.StatusUnprocessableEntity, "source_id, field_group and policy are required and must be known values")
		return
	}
	q := db.New(s.pool)
	if _, err := q.GetSource(r.Context(), in.SourceID); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "unknown source")
		return
	}
	var err error
	if in.Policy == catalog.PolicyUse && in.Brand == "" {
		_, err = q.DeleteUsagePolicy(r.Context(), db.DeleteUsagePolicyParams{SourceID: in.SourceID, Brand: "", FieldGroup: in.Group})
	} else {
		err = q.UpsertUsagePolicy(r.Context(), db.UpsertUsagePolicyParams{SourceID: in.SourceID, Brand: in.Brand, FieldGroup: in.Group, Policy: in.Policy})
	}
	if err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteUsagePolicy removes the entry for a brand, so the source-wide
// setting applies to it again.
func (s *server) deleteUsagePolicy(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	n, err := db.New(s.pool).DeleteUsagePolicy(r.Context(), db.DeleteUsagePolicyParams{
		SourceID: query.Get("source_id"), Brand: query.Get("brand"), FieldGroup: query.Get("field_group")})
	if err != nil {
		internalError(w, err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
