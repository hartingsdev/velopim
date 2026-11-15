// Package fetch downloads a supplier's file from a web address, so that
// velopim can collect deliveries itself instead of waiting for an upload.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Kinds of authentication.
const (
	AuthNone   = ""
	AuthBasic  = "basic"  // user name and password
	AuthBearer = "bearer" // "Authorization: Bearer <token>"
	AuthHeader = "header" // a named header with a secret value, e.g. X-API-Key
)

// Config says where a source's file is and how often to fetch it. Secrets
// are never part of it: it only names the environment variables that hold
// them.
type Config struct {
	// URL is the http or https address of the file. Empty: nothing to fetch.
	URL string `json:"url"`
	// IntervalMinutes is how often to fetch; 0 means only on request.
	IntervalMinutes int    `json:"interval_minutes"`
	Auth            string `json:"auth"`
	// UserEnv and SecretEnv name environment variables. For basic auth both
	// are used; for bearer and header only SecretEnv.
	UserEnv   string `json:"user_env,omitempty"`
	SecretEnv string `json:"secret_env,omitempty"`
	// HeaderName is the header for AuthHeader.
	HeaderName string `json:"header_name,omitempty"`
}

// EnvPrefix must start the name of every environment variable a Config
// refers to. Without it a configuration could send any secret of the
// process, such as the database password, to a foreign server.
const EnvPrefix = "VELOPIM_FETCH_"

var (
	envNamePattern    = regexp.MustCompile(`^VELOPIM_FETCH_[A-Z0-9_]+$`)
	headerNamePattern = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
)

// MinIntervalMinutes keeps a misconfiguration from hammering a supplier.
const MinIntervalMinutes = 5

// Enabled reports whether there is anything to fetch.
func (c Config) Enabled() bool { return strings.TrimSpace(c.URL) != "" }

// Validate reports the first reason the configuration cannot be used. An
// empty configuration is valid.
func (c Config) Validate() error {
	if !c.Enabled() {
		if c.IntervalMinutes != 0 {
			return errors.New("fetch: an interval needs an address")
		}
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(c.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("fetch: the address must start with http:// or https://")
	}
	if u.User != nil {
		return errors.New("fetch: do not put user name or password into the address; use the authentication fields")
	}
	if c.IntervalMinutes != 0 && c.IntervalMinutes < MinIntervalMinutes {
		return fmt.Errorf("fetch: the interval must be at least %d minutes, or 0 for on request only", MinIntervalMinutes)
	}
	env := func(what, name string) error {
		if !envNamePattern.MatchString(name) {
			return fmt.Errorf("fetch: %s must name an environment variable starting with %s", what, EnvPrefix)
		}
		return nil
	}
	switch c.Auth {
	case AuthNone:
	case AuthBasic:
		return errors.Join(env("user_env", c.UserEnv), env("secret_env", c.SecretEnv))
	case AuthBearer:
		return env("secret_env", c.SecretEnv)
	case AuthHeader:
		if !headerNamePattern.MatchString(c.HeaderName) {
			return errors.New("fetch: header_name is required for header authentication")
		}
		return env("secret_env", c.SecretEnv)
	default:
		return fmt.Errorf("fetch: unknown authentication %q", c.Auth)
	}
	return nil
}

// MaxSize bounds a downloaded file.
const MaxSize = 256 << 20

// Timeout bounds one download.
const Timeout = 5 * time.Minute

// Get downloads the file. getenv looks up the secrets (os.Getenv); client
// may be nil for the default.
func Get(ctx context.Context, c Config, client *http.Client, getenv func(string) string) ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if !c.Enabled() {
		return nil, errors.New("fetch: no address configured")
	}
	if client == nil {
		client = &http.Client{}
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(c.URL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "velopim")
	secret := func(name string) (string, error) {
		v := getenv(name)
		if v == "" {
			return "", fmt.Errorf("fetch: environment variable %s is not set", name)
		}
		return v, nil
	}
	switch c.Auth {
	case AuthBasic:
		user, err1 := secret(c.UserEnv)
		pass, err2 := secret(c.SecretEnv)
		if err := errors.Join(err1, err2); err != nil {
			return nil, err
		}
		req.SetBasicAuth(user, pass)
	case AuthBearer:
		token, err := secret(c.SecretEnv)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	case AuthHeader:
		value, err := secret(c.SecretEnv)
		if err != nil {
			return nil, err
		}
		req.Header.Set(c.HeaderName, value)
	}

	res, err := client.Do(req)
	if err != nil {
		// The error text of net/http contains the address but no headers.
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch: the server answered with status %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, MaxSize+1))
	if err != nil {
		return nil, fmt.Errorf("fetch: reading the file: %w", err)
	}
	if len(data) > MaxSize {
		return nil, errors.New("fetch: the file is larger than 256 MB")
	}
	if len(data) == 0 {
		return nil, errors.New("fetch: the server sent an empty file")
	}
	return data, nil
}
