package fetch

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name  string
		cfg   Config
		valid bool
	}{
		{"empty: nothing to fetch", Config{}, true},
		{"plain address", Config{URL: "https://example.test/feed.csv"}, true},
		{"with interval", Config{URL: "https://example.test/feed.csv", IntervalMinutes: 60}, true},
		{"basic auth", Config{URL: "https://example.test/f", Auth: AuthBasic, UserEnv: "VELOPIM_FETCH_HARTJE_USER", SecretEnv: "VELOPIM_FETCH_HARTJE_PASSWORD"}, true},
		{"bearer", Config{URL: "https://example.test/f", Auth: AuthBearer, SecretEnv: "VELOPIM_FETCH_TOKEN"}, true},
		{"header", Config{URL: "https://example.test/f", Auth: AuthHeader, HeaderName: "X-API-Key", SecretEnv: "VELOPIM_FETCH_KEY"}, true},
		{"interval without address", Config{IntervalMinutes: 60}, false},
		{"not a web address", Config{URL: "ftp://example.test/feed.csv"}, false},
		{"local file", Config{URL: "file:///etc/passwd"}, false},
		{"no host", Config{URL: "https:///feed.csv"}, false},
		{"credentials in the address", Config{URL: "https://user:pass@example.test/f"}, false},
		{"interval too short", Config{URL: "https://example.test/f", IntervalMinutes: 1}, false},
		{"unknown auth", Config{URL: "https://example.test/f", Auth: "magic"}, false},
		{"basic without user variable", Config{URL: "https://example.test/f", Auth: AuthBasic, SecretEnv: "VELOPIM_FETCH_P"}, false},
		{"variable outside the allowed prefix", Config{URL: "https://example.test/f", Auth: AuthBearer, SecretEnv: "SHOPIFY_CLIENT_SECRET"}, false},
		{"variable that only contains the prefix", Config{URL: "https://example.test/f", Auth: AuthBearer, SecretEnv: "X_VELOPIM_FETCH_TOKEN"}, false},
		{"header without name", Config{URL: "https://example.test/f", Auth: AuthHeader, SecretEnv: "VELOPIM_FETCH_KEY"}, false},
		{"header name with line break", Config{URL: "https://example.test/f", Auth: AuthHeader, HeaderName: "X-Key\r\nHost", SecretEnv: "VELOPIM_FETCH_KEY"}, false},
	}
	for _, c := range cases {
		if err := c.cfg.Validate(); (err == nil) != c.valid {
			t.Errorf("%s: Validate() = %v, want valid=%v", c.name, err, c.valid)
		}
	}
}

func TestGet(t *testing.T) {
	env := map[string]string{"VELOPIM_FETCH_USER": "hartings", "VELOPIM_FETCH_PASSWORD": "geheim", "VELOPIM_FETCH_TOKEN": "tok-123"}
	getenv := func(k string) string { return env[k] }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open":
			_, _ = io.WriteString(w, "a;b\n1;2\n")
		case "/basic":
			if u, p, ok := r.BasicAuth(); !ok || u != "hartings" || p != "geheim" {
				http.Error(w, "no", http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, "basic ok")
		case "/bearer":
			if r.Header.Get("Authorization") != "Bearer tok-123" {
				http.Error(w, "no", http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, "bearer ok")
		case "/header":
			if r.Header.Get("X-API-Key") != "tok-123" {
				http.Error(w, "no", http.StatusForbidden)
				return
			}
			_, _ = io.WriteString(w, "header ok")
		case "/empty":
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cases := []struct {
		name    string
		cfg     Config
		want    string // body, or empty when an error is expected
		wantErr string
	}{
		{"no auth", Config{URL: srv.URL + "/open"}, "a;b\n1;2\n", ""},
		{"basic", Config{URL: srv.URL + "/basic", Auth: AuthBasic, UserEnv: "VELOPIM_FETCH_USER", SecretEnv: "VELOPIM_FETCH_PASSWORD"}, "basic ok", ""},
		{"bearer", Config{URL: srv.URL + "/bearer", Auth: AuthBearer, SecretEnv: "VELOPIM_FETCH_TOKEN"}, "bearer ok", ""},
		{"header", Config{URL: srv.URL + "/header", Auth: AuthHeader, HeaderName: "X-API-Key", SecretEnv: "VELOPIM_FETCH_TOKEN"}, "header ok", ""},
		{"missing credentials are refused by the server", Config{URL: srv.URL + "/basic"}, "", "status 401"},
		{"variable not set", Config{URL: srv.URL + "/bearer", Auth: AuthBearer, SecretEnv: "VELOPIM_FETCH_MISSING"}, "", "VELOPIM_FETCH_MISSING is not set"},
		{"not found", Config{URL: srv.URL + "/nowhere"}, "", "status 404"},
		{"empty file", Config{URL: srv.URL + "/empty"}, "", "empty file"},
		{"nothing configured", Config{}, "", "no address"},
	}
	for _, c := range cases {
		data, err := Get(context.Background(), c.cfg, srv.Client(), getenv)
		switch {
		case c.wantErr == "" && (err != nil || string(data) != c.want):
			t.Errorf("%s: got %q, %v", c.name, data, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: error %v, want it to contain %q", c.name, err, c.wantErr)
		}
		// A secret must never show up in an error message.
		if err != nil && (strings.Contains(err.Error(), "geheim") || strings.Contains(err.Error(), "tok-123")) {
			t.Errorf("%s: error leaks a secret: %v", c.name, err)
		}
	}
}
