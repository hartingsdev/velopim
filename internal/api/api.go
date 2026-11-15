// Package api is the JSON API the web interface talks to.
//
// There is no login yet: the server must only listen on localhost. Writing
// requests must be JSON, which keeps other websites from triggering them
// through the browser (a cross-site form cannot send that content type).
package api

import (
	"encoding/json"
	"errors"
	"log"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"velopim/internal/catalog"
	"velopim/internal/db"
	"velopim/internal/tasks"
)

// listLimit caps list responses; the counts tell the client when there is more.
const listLimit = 200

// Task is one entry of the task list.
type Task struct {
	ID          int64       `json:"id"`
	Kind        string      `json:"kind"`
	Title       string      `json:"title"`
	Detail      string      `json:"detail"`
	Status      string      `json:"status"`
	Priority    int         `json:"priority"`
	CreatedAt   time.Time   `json:"created_at"`
	ClosedAt    *time.Time  `json:"closed_at"`
	ClosedBy    *string     `json:"closed_by"`
	Note        *string     `json:"note"`
	ImportRunID *int64      `json:"import_run_id"`
	Action      *TaskAction `json:"action"`
	Item        *TaskItem   `json:"item"`
	// Variants lists the supplier items of a new-model task.
	Variants []TaskItem `json:"variants"`
	// LockedSplitBy is set when the model already has products: their cut
	// applies and cannot be chosen again.
	LockedSplitBy []string `json:"locked_split_by"`
}

// TaskAction is the proposed change a task asks to approve.
type TaskAction struct {
	Type   string  `json:"type"`
	Field  string  `json:"field"`
	Old    string  `json:"old"`
	New    string  `json:"new"`
	Reason string  `json:"reason"`
	Rule   *string `json:"rule"`
	// For a purchase price change: the own sales price now and what
	// approving sets it to. Null when the article has no priced variant.
	BasePriceCents     *int64 `json:"base_price_cents"`
	ProposedPriceCents *int64 `json:"proposed_price_cents"`
}

// TaskItem is the supplier item a task is about.
type TaskItem struct {
	SourceID  string `json:"source_id"`
	SKU       string `json:"sku"`
	Brand     string `json:"brand"`
	Name      string `json:"name"`
	ModelYear *int32 `json:"model_year"`
	Color     string `json:"color"`
	FrameSize string `json:"frame_size"`
	// Only filled for the variants of a new-model task. PriceCents is what
	// the matching pricing rule calculates; null without a rule.
	Category   string            `json:"category"`
	CostCents  *int64            `json:"cost_cents"`
	RRPCents   *int64            `json:"rrp_cents"`
	PriceCents *int64            `json:"price_cents"`
	Attributes map[string]string `json:"attributes"`
}

// TaskList is the response of GET /api/tasks.
type TaskList struct {
	Tasks  []Task `json:"tasks"`
	Open   int64  `json:"open"`
	Closed int64  `json:"closed"`
}

// ImportRun is one entry of GET /api/import-runs.
type ImportRun struct {
	ID          int64           `json:"id"`
	SourceID    string          `json:"source_id"`
	SourceName  string          `json:"source_name"`
	Status      string          `json:"status"`
	RecordCount int32           `json:"record_count"`
	IssueCount  int32           `json:"issue_count"`
	Summary     json.RawMessage `json:"summary"`
	Error       *string         `json:"error"`
	StartedAt   time.Time       `json:"started_at"`
	FinishedAt  *time.Time      `json:"finished_at"`
}

// Decision is the body of POST /api/tasks/{id}/decision.
type Decision struct {
	Approve bool   `json:"approve"`
	Note    string `json:"note"`
	// SplitBy lists the axes that separate products when a new model is
	// approved: "color", "frame_size" or "attributes.<key>".
	SplitBy []string `json:"split_by"`
}

type server struct {
	pool  *pgxpool.Pool
	actor string
}

// New returns the handler for everything under /api. actor is recorded as
// the person behind every decision.
func New(pool *pgxpool.Pool, actor string) http.Handler {
	s := &server{pool: pool, actor: actor}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/tasks", s.listTasks)
	mux.HandleFunc("POST /api/tasks/{id}/decision", s.decideTask)
	mux.HandleFunc("GET /api/import-runs", s.listImportRuns)
	mux.HandleFunc("GET /api/products", s.listProducts)
	mux.HandleFunc("GET /api/pricing-rules", s.listPricingRules)
	mux.HandleFunc("PUT /api/pricing-rules", s.putPricingRule)
	mux.HandleFunc("DELETE /api/pricing-rules/{id}", s.deletePricingRule)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	return mux
}

func (s *server) listTasks(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.pool)
	var rows []db.ListOpenTasksRow
	var err error
	switch r.URL.Query().Get("status") {
	case "", "open":
		rows, err = q.ListOpenTasks(r.Context(), listLimit)
	case "closed":
		var closed []db.ListClosedTasksRow
		closed, err = q.ListClosedTasks(r.Context(), listLimit)
		for _, c := range closed {
			rows = append(rows, db.ListOpenTasksRow(c))
		}
	default:
		writeError(w, http.StatusBadRequest, "status must be open or closed")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	counts, err := q.CountTasks(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	out := TaskList{Tasks: make([]Task, 0, len(rows)), Open: counts.Open, Closed: counts.Closed}
	for _, row := range rows {
		out.Tasks = append(out.Tasks, taskFromRow(row))
	}
	if err := s.addModelDetails(r.Context(), q, rows, out.Tasks); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func taskFromRow(row db.ListOpenTasksRow) Task {
	t := Task{
		ID: row.ID, Kind: row.Kind, Title: row.Title, Detail: row.Detail, Status: row.Status,
		Priority: int(row.Priority), CreatedAt: row.CreatedAt, ClosedAt: row.ClosedAt, ClosedBy: row.ClosedBy,
		Note: row.Note, ImportRunID: row.ImportRunID, Variants: []TaskItem{},
	}
	if row.ActionType != nil {
		t.Action = &TaskAction{Type: *row.ActionType, Field: str(row.ActionField), Old: str(row.OldValue),
			New: str(row.NewValue), Reason: str(row.DecisionReason), Rule: row.RuleName,
			BasePriceCents: row.BasePriceCents, ProposedPriceCents: row.ProposedPriceCents}
	}
	if row.Sku != nil {
		t.Item = &TaskItem{SourceID: str(row.SourceID), SKU: *row.Sku, Brand: str(row.Brand), Name: str(row.ItemName),
			ModelYear: row.ModelYear, Color: str(row.Color), FrameSize: str(row.FrameSize)}
	}
	return t
}

func (s *server) decideTask(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}
	var d Decision
	if !readJSON(w, r, &d) {
		return
	}
	splitBy := make([]catalog.Axis, len(d.SplitBy))
	for i, a := range d.SplitBy {
		splitBy[i] = catalog.Axis(a)
	}
	err = tasks.Decide(r.Context(), s.pool, id, d.Approve, s.actor, d.Note, splitBy)
	var blocked *catalog.BlockedError
	if errors.As(err, &blocked) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "model cannot be created as cut", "issues": blocked.Issues})
		return
	}
	if errors.Is(err, tasks.ErrNotOpen) {
		writeError(w, http.StatusConflict, "task is not open")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listImportRuns(w http.ResponseWriter, r *http.Request) {
	rows, err := db.New(s.pool).ListImportRuns(r.Context(), 50)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]ImportRun, 0, len(rows))
	for _, row := range rows {
		out = append(out, ImportRun{ID: row.ID, SourceID: row.SourceID, SourceName: row.SourceName, Status: row.Status,
			RecordCount: row.RecordCount, IssueCount: row.IssueCount, Summary: row.Summary, Error: row.Error,
			StartedAt: row.StartedAt, FinishedAt: row.FinishedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": out})
}

// readJSON decodes a JSON request body. It answers the request itself and
// returns false when the body is not acceptable.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "content type must be application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// internalError logs the cause and hides it from the client.
func internalError(w http.ResponseWriter, err error) {
	log.Printf("api: %v", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}
