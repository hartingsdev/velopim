package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"velopim/internal/db"
	"velopim/internal/rules"
)

// RuleList is the response of GET /api/rules. Sources are included so that
// the form can offer them as scope.
type RuleList struct {
	Rules   []rules.Rule `json:"rules"`
	Sources []Source     `json:"sources"`
}

// Source is a supplier or feed.
type Source struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *server) listRules(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.pool)
	rows, err := q.ListRules(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	out := RuleList{Rules: make([]rules.Rule, 0, len(rows)), Sources: []Source{}}
	for _, row := range rows {
		rule := rules.Rule{ID: row.ID, Name: row.Name, Enabled: row.Enabled, ActionType: rules.ActionType(row.ActionType),
			Scope:   rules.Scope{SourceID: row.ScopeSourceID, Brand: row.ScopeBrand, Category: row.ScopeCategory},
			Outcome: rules.Outcome(row.Outcome), Guard: row.Guard, Conditions: []rules.Condition{}}
		if err := json.Unmarshal(row.Conditions, &rule.Conditions); err != nil {
			internalError(w, err)
			return
		}
		out.Rules = append(out.Rules, rule)
	}
	sources, err := q.ListSources(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	for _, src := range sources {
		out.Sources = append(out.Sources, Source{ID: src.ID, Name: src.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

// readRule decodes and validates a rule from the request body.
func readRule(w http.ResponseWriter, r *http.Request) (rules.Rule, []byte, bool) {
	var rule rules.Rule
	if !readJSON(w, r, &rule) {
		return rule, nil, false
	}
	if err := rule.Validate(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return rule, nil, false
	}
	if rule.Conditions == nil {
		rule.Conditions = []rules.Condition{}
	}
	conditions, _ := json.Marshal(rule.Conditions)
	return rule, conditions, true
}

func (s *server) createRule(w http.ResponseWriter, r *http.Request) {
	rule, conditions, ok := readRule(w, r)
	if !ok {
		return
	}
	id, err := db.New(s.pool).InsertRule(r.Context(), db.InsertRuleParams{
		Name: rule.Name, Enabled: rule.Enabled, ActionType: string(rule.ActionType), ScopeSourceID: rule.Scope.SourceID,
		ScopeBrand: rule.Scope.Brand, ScopeCategory: rule.Scope.Category, Conditions: conditions,
		Outcome: string(rule.Outcome), Guard: rule.Guard})
	if err != nil {
		internalError(w, err)
		return
	}
	rule.ID = id
	writeJSON(w, http.StatusCreated, rule)
}

func (s *server) updateRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	rule, conditions, ok := readRule(w, r)
	if !ok {
		return
	}
	n, err := db.New(s.pool).UpdateRule(r.Context(), db.UpdateRuleParams{
		ID: id, Name: rule.Name, Enabled: rule.Enabled, ActionType: string(rule.ActionType),
		ScopeSourceID: rule.Scope.SourceID, ScopeBrand: rule.Scope.Brand, ScopeCategory: rule.Scope.Category,
		Conditions: conditions, Outcome: string(rule.Outcome), Guard: rule.Guard})
	if err != nil {
		internalError(w, err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	rule.ID = id
	writeJSON(w, http.StatusOK, rule)
}

// deleteRule removes a rule that never decided anything. A rule that
// actions refer to stays for the record and can only be disabled.
func (s *server) deleteRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	q := db.New(s.pool)
	used, err := q.RuleIsUsed(r.Context(), id)
	if err != nil {
		internalError(w, err)
		return
	}
	if used {
		writeError(w, http.StatusConflict, "rule has decided actions; disable it instead")
		return
	}
	n, err := q.DeleteRule(r.Context(), id)
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
