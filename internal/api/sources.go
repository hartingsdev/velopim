package api

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"

	"velopim/internal/availability"
	"velopim/internal/connector/csvfile"
	"velopim/internal/db"
)

// SourceConfig is a supplier with how its files are read.
type SourceConfig struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Connector string `json:"connector_kind"`
	// Config is the CSV layout; AvailabilityMapping the ordered rules that
	// turn raw fields into an availability status.
	Config              csvfile.Config             `json:"connector_config"`
	AvailabilityMapping []availability.MappingRule `json:"availability_mapping"`
}

var sourceIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

func (s *server) listSources(w http.ResponseWriter, r *http.Request) {
	rows, err := db.New(s.pool).ListSourcesFull(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]SourceConfig, 0, len(rows))
	for _, row := range rows {
		src := SourceConfig{ID: row.ID, Name: row.Name, Connector: row.ConnectorKind, AvailabilityMapping: []availability.MappingRule{}}
		// A configuration that cannot be read is shown empty and can be
		// replaced by saving.
		_ = json.Unmarshal(row.ConnectorConfig, &src.Config)
		_ = json.Unmarshal(row.AvailabilityMapping, &src.AvailabilityMapping)
		if src.Config.Columns == nil {
			src.Config.Columns = map[string]string{}
		}
		out = append(out, src)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": out})
}

// putSource creates a supplier or replaces its configuration. Only CSV
// files are supported so far.
func (s *server) putSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !sourceIDPattern.MatchString(id) {
		writeError(w, http.StatusUnprocessableEntity, "id must consist of lowercase letters, digits and hyphens")
		return
	}
	var in struct {
		Name                string                     `json:"name"`
		Config              csvfile.Config             `json:"connector_config"`
		AvailabilityMapping []availability.MappingRule `json:"availability_mapping"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeError(w, http.StatusUnprocessableEntity, "name is required")
		return
	}
	in.Config.SourceID = id
	if _, err := csvfile.New(in.Config); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := availability.ValidateRules(in.AvailabilityMapping); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if in.AvailabilityMapping == nil {
		in.AvailabilityMapping = []availability.MappingRule{}
	}
	in.Config.SourceID = "" // the id is the row's key, not part of the stored layout
	config, _ := json.Marshal(in.Config)
	mapping, _ := json.Marshal(in.AvailabilityMapping)
	if err := db.New(s.pool).UpsertSource(r.Context(), db.UpsertSourceParams{ID: id, Name: strings.TrimSpace(in.Name),
		ConnectorKind: "csvfile", ConnectorConfig: config, AvailabilityMapping: mapping}); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// previewCSV reads the header and first rows of a file sent as the request
// body, so that its columns can be mapped. Nothing is stored.
func (s *server) previewCSV(w http.ResponseWriter, r *http.Request) {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/octet-stream" {
		writeError(w, http.StatusUnsupportedMediaType, "content type must be application/octet-stream")
		return
	}
	// The start of the file is enough.
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "unreadable body")
		return
	}
	delimiter := r.URL.Query().Get("delimiter")
	if delimiter == "" {
		delimiter = guessDelimiter(data)
	}
	header, rows, encoding, err := csvfile.Preview(data, delimiter, r.URL.Query().Get("encoding"), 5)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if rows == nil {
		rows = [][]string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"delimiter": delimiter, "encoding": encoding, "columns": header, "rows": rows})
}

// guessDelimiter picks the separator that occurs most often in the first line.
func guessDelimiter(data []byte) string {
	line, _, _ := strings.Cut(string(data), "\n")
	best, count := ";", 0
	for _, d := range []string{";", ",", "\t", "|"} {
		if n := strings.Count(line, d); n > count {
			best, count = d, n
		}
	}
	return best
}
