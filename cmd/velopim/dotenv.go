package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// loadDotEnv reads KEY=VALUE lines from a file into the environment. A
// variable that is already set keeps its value, so a setting given on the
// command line wins over the file. A missing file is not an error.
//
// Supported: blank lines, comments starting with #, an optional "export "
// prefix, and values in single or double quotes. Nothing is expanded.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		key, value, ok, err := parseEnvLine(scanner.Text())
		if err != nil {
			return fmt.Errorf("%s, line %d: %w", path, line, err)
		}
		if !ok {
			continue
		}
		if _, set := os.LookupEnv(key); !set {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("%s, line %d: %w", path, line, err)
			}
		}
	}
	return scanner.Err()
}

// parseEnvLine splits one line. ok is false for blank lines and comments.
func parseEnvLine(text string) (key, value string, ok bool, err error) {
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "#") {
		return "", "", false, nil
	}
	text = strings.TrimSpace(strings.TrimPrefix(text, "export "))
	key, value, found := strings.Cut(text, "=")
	key = strings.TrimSpace(key)
	if !found || key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false, fmt.Errorf("expected KEY=VALUE")
	}
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
		value = value[1 : len(value)-1]
	}
	return key, value, true, nil
}
