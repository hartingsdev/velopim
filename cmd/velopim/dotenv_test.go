package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvLine(t *testing.T) {
	cases := []struct {
		line, key, value string
		ok, wantErr      bool
	}{
		{"KEY=value", "KEY", "value", true, false},
		{"  KEY = value with spaces  ", "KEY", "value with spaces", true, false},
		{"export KEY=value", "KEY", "value", true, false},
		{`KEY="quoted value"`, "KEY", "quoted value", true, false},
		{"KEY='single'", "KEY", "single", true, false},
		{"KEY=", "KEY", "", true, false},
		{"URL=postgres://u:p@host/db?sslmode=disable", "URL", "postgres://u:p@host/db?sslmode=disable", true, false},
		{"KEY=a#b", "KEY", "a#b", true, false}, // a # inside a value belongs to it, e.g. in a password
		{"", "", "", false, false},
		{"   ", "", "", false, false},
		{"# comment", "", "", false, false},
		{"no equals sign", "", "", false, true},
		{"=value", "", "", false, true},
		{"TWO WORDS=value", "", "", false, true},
	}
	for _, c := range cases {
		key, value, ok, err := parseEnvLine(c.line)
		if key != c.key || value != c.value || ok != c.ok || (err != nil) != c.wantErr {
			t.Errorf("%q: got (%q, %q, %v, %v), want (%q, %q, %v, error=%v)", c.line, key, value, ok, err, c.key, c.value, c.ok, c.wantErr)
		}
	}
}

func TestLoadDotEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("# settings\nVELOPIM_TEST_FROM_FILE=file\nVELOPIM_TEST_ALREADY_SET=file\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VELOPIM_TEST_ALREADY_SET", "environment")
	os.Unsetenv("VELOPIM_TEST_FROM_FILE")
	t.Cleanup(func() { os.Unsetenv("VELOPIM_TEST_FROM_FILE") })

	if err := loadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("VELOPIM_TEST_FROM_FILE"); got != "file" {
		t.Errorf("value from the file = %q", got)
	}
	// What is already set, e.g. on the command line, wins over the file.
	if got := os.Getenv("VELOPIM_TEST_ALREADY_SET"); got != "environment" {
		t.Errorf("already set value was overwritten: %q", got)
	}
	if err := loadDotEnv(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Errorf("a missing file must not be an error: %v", err)
	}
	bad := filepath.Join(t.TempDir(), "bad")
	_ = os.WriteFile(bad, []byte("OK=1\nbroken line\n"), 0o600)
	if err := loadDotEnv(bad); err == nil {
		t.Error("a broken line was accepted")
	}
}
