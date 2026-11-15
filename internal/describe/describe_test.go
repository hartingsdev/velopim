package describe

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

func TestPrompt(t *testing.T) {
	tests := []struct {
		name    string
		in      Input
		want    []string
		wantNot []string
	}{
		{
			name: "all facts",
			in: Input{Brand: "Beispielrad", Name: "City 3 Wave", ModelYear: 2027, Category: "Cityrad",
				Options:      []Option{{Name: "Rahmenhöhe", Values: []string{"45", "50"}}},
				Attributes:   map[string][]string{"Schaltung": {"Nexus 7"}, "Bremse": {"V-Brake", "Rücktritt"}, "Leer": {}},
				SupplierText: "  Ein Rad für die Stadt.  "},
			want: []string{"Marke: Beispielrad\n", "Modell: City 3 Wave\n", "Modelljahr: 2027\n", "Kategorie: Cityrad\n",
				"Wählbar – Rahmenhöhe: 45, 50\n", "Bremse: V-Brake / Rücktritt\nSchaltung: Nexus 7\n",
				"<lieferantentext>\nEin Rad für die Stadt.\n</lieferantentext>"},
			wantNot: []string{"Leer"},
		},
		{
			name:    "only a name",
			in:      Input{Name: "Gravel 2"},
			want:    []string{"Modell: Gravel 2\n"},
			wantNot: []string{"Marke", "Modelljahr", "<technische_daten>", "<lieferantentext>"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Prompt(tc.in)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("prompt lacks %q:\n%s", w, got)
				}
			}
			for _, w := range tc.wantNot {
				if strings.Contains(got, w) {
					t.Errorf("prompt contains %q:\n%s", w, got)
				}
			}
			if got != Prompt(tc.in) {
				t.Error("prompt is not stable")
			}
		})
	}
}

func TestParseAndHTML(t *testing.T) {
	tests := []struct {
		name    string
		answer  string
		want    string
		wantErr bool
	}{
		{name: "paragraphs and list", answer: `{"paragraphs":["Eins.","Zwei."],"highlights":["Nexus 7","LED-Licht"]}`,
			want: "<p>Eins.</p>\n<p>Zwei.</p>\n<ul>\n  <li>Nexus 7</li>\n  <li>LED-Licht</li>\n</ul>"},
		{name: "no highlights, blanks dropped", answer: `{"paragraphs":[" Eins. ",""],"highlights":[" "]}`, want: "<p>Eins.</p>"},
		{name: "markup is escaped", answer: `{"paragraphs":["<script>alert(1)</script> 28\" & mehr"],"highlights":[]}`,
			want: "<p>&lt;script&gt;alert(1)&lt;/script&gt; 28&#34; &amp; mehr</p>"},
		{name: "empty", answer: `{"paragraphs":[],"highlights":["x"]}`, wantErr: true},
		{name: "not json", answer: `Hier ist der Text`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse(tc.answer)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error: %v", err, tc.wantErr)
			}
			if err == nil && d.HTML() != tc.want {
				t.Errorf("got\n%s\nwant\n%s", d.HTML(), tc.want)
			}
		})
	}
}

// fakeClaude answers a Messages request and records what was asked.
func fakeClaude(t *testing.T, status int, body string, seen *map[string]any) *Generator {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected request %s (authorization %q)", r.URL.Path, r.Header.Get("Authorization"))
		}
		if seen != nil {
			_ = json.NewDecoder(r.Body).Decode(seen)
			(*seen)["beta-header"] = r.Header.Get("Anthropic-Beta")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return New("test-model", option.WithAPIKey("test-key"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
}

func message(stop, text string) string {
	b, _ := json.Marshal(map[string]any{"id": "msg_1", "type": "message", "role": "assistant", "model": "test-model",
		"stop_reason": stop, "content": []map[string]any{{"type": "text", "text": text}},
		"usage": map[string]any{"input_tokens": 1, "output_tokens": 1}})
	return string(b)
}

func TestGenerate(t *testing.T) {
	ctx := context.Background()
	in := Input{Brand: "Beispielrad", Name: "City 3 Wave"}

	seen := map[string]any{}
	g := fakeClaude(t, 200, message("end_turn", `{"paragraphs":["Ein Stadtrad."],"highlights":["Nexus 7"]}`), &seen)
	d, err := g.Generate(ctx, in)
	if err != nil || len(d.Paragraphs) != 1 || d.Highlights[0] != "Nexus 7" {
		t.Fatalf("draft %+v, err %v", d, err)
	}
	if seen["model"] != "test-model" || seen["fallbacks"] != "default" || !strings.Contains(seen["beta-header"].(string), "server-side-fallback") {
		t.Errorf("request: model %v, fallbacks %v, beta %v", seen["model"], seen["fallbacks"], seen["beta-header"])
	}
	cfg, _ := seen["output_config"].(map[string]any)
	format, _ := cfg["format"].(map[string]any)
	if cfg["effort"] != "low" || format["type"] != "json_schema" || format["schema"] == nil {
		t.Errorf("output_config = %v", cfg)
	}
	sent, _ := json.Marshal(seen["messages"])
	if !strings.Contains(string(sent), "City 3 Wave") {
		t.Errorf("the product is missing from the request: %s", sent)
	}

	if _, err := fakeClaude(t, 200, message("refusal", ""), nil).Generate(ctx, in); !errors.Is(err, ErrRefused) {
		t.Errorf("refusal: got %v", err)
	}
	if _, err := fakeClaude(t, 200, message("max_tokens", `{"paragraphs":["Ein`), nil).Generate(ctx, in); err == nil {
		t.Error("cut-off answer was accepted")
	}
	_, err = fakeClaude(t, 401, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`, nil).Generate(ctx, in)
	if err == nil || !strings.Contains(err.Error(), "API key") || strings.Contains(err.Error(), "test-key") {
		t.Errorf("401: got %v", err)
	}
}

func TestFromEnv(t *testing.T) {
	if _, ok := FromEnv(func(string) string { return "" }); ok {
		t.Error("generator without key")
	}
	g, ok := FromEnv(func(k string) string { return map[string]string{"ANTHROPIC_API_KEY": "k"}[k] })
	if !ok || g.Model() != DefaultModel {
		t.Errorf("default model: %v %v", ok, g)
	}
	g, _ = FromEnv(func(k string) string {
		return map[string]string{"ANTHROPIC_API_KEY": "k", "VELOPIM_AI_MODEL": "claude-haiku-5-5"}[k]
	})
	if g.Model() != "claude-haiku-5-5" {
		t.Errorf("model = %s", g.Model())
	}
}
