package shopify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// APIVersion is the Admin API version all requests use.
const APIVersion = "2026-10"

// Config holds what the client needs. All values come from the environment.
type Config struct {
	// StoreDomain is the store's myshopify.com domain.
	StoreDomain  string
	ClientID     string
	ClientSecret string
	// WriteEnabled must be true for any mutation (SHOPIFY_WRITE_ENABLED).
	WriteEnabled bool
}

var storeDomainPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.myshopify\.com$`)

// Client talks to the Admin GraphQL API of one store. It gets its access
// token with the client credentials grant and renews it before it expires.
type Client struct {
	cfg  Config
	http *http.Client
	// base is "https://<store domain>"; tests point it at a fake server.
	base string

	mu           sync.Mutex
	token        string
	tokenExpires time.Time
	scopes       []string
	devStore     *bool // cached answer to "is this a development store?"
	location     string
	onlineStore  string // publication id of the online store channel
}

// NewClient validates the configuration. It makes no request.
func NewClient(cfg Config) (*Client, error) {
	cfg.StoreDomain = strings.ToLower(strings.TrimSpace(cfg.StoreDomain))
	if !storeDomainPattern.MatchString(cfg.StoreDomain) {
		return nil, fmt.Errorf("shopify: store domain %q is not a myshopify.com domain", cfg.StoreDomain)
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("shopify: client id and client secret are required")
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: 30 * time.Second}, base: "https://" + cfg.StoreDomain}, nil
}

// ErrWritesDisabled is returned for a mutation while writing is switched off.
var ErrWritesDisabled = errors.New("shopify: writing is disabled (SHOPIFY_WRITE_ENABLED is not true)")

// ErrNotDevStore is returned for a mutation against a store that Shopify
// does not report as a development store.
var ErrNotDevStore = errors.New("shopify: refusing to write: the store is not a development store")

// accessToken returns a valid token, requesting a new one when needed.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExpires.Add(-time.Minute)) {
		return c.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {c.cfg.ClientID}, "client_secret": {c.cfg.ClientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/admin/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("shopify: requesting access token: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		// The body of a failed token request names the reason and holds no secret.
		return "", fmt.Errorf("shopify: access token request failed with status %d: %s", res.StatusCode, truncate(string(body), 300))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		Scope       string `json:"scope"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" {
		return "", errors.New("shopify: access token response has no token")
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 3600
	}
	c.token, c.tokenExpires = out.AccessToken, time.Now().Add(time.Duration(out.ExpiresIn)*time.Second)
	c.scopes = strings.Split(out.Scope, ",")
	return c.token, nil
}

// Scopes returns the access scopes of the current token.
func (c *Client) Scopes(ctx context.Context) ([]string, error) {
	if _, err := c.accessToken(ctx); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{}, c.scopes...), nil
}

// GraphQLError is an error the API returned for a query.
type GraphQLError struct {
	Messages []string
}

func (e *GraphQLError) Error() string { return "shopify: " + strings.Join(e.Messages, "; ") }

// maxAttempts bounds retries of throttled or failed requests.
const maxAttempts = 5

// Query runs a read-only GraphQL document.
func (c *Client) Query(ctx context.Context, query string, variables map[string]any, out any) error {
	if isMutation(query) {
		return errors.New("shopify: Query must not be used for mutations")
	}
	return c.do(ctx, query, variables, out)
}

// Mutate runs a GraphQL mutation. It is refused unless writing is enabled
// and Shopify reports the store as a development store.
func (c *Client) Mutate(ctx context.Context, query string, variables map[string]any, out any) error {
	if !c.cfg.WriteEnabled {
		return ErrWritesDisabled
	}
	dev, err := c.IsDevStore(ctx)
	if err != nil {
		return err
	}
	if !dev {
		return ErrNotDevStore
	}
	return c.do(ctx, query, variables, out)
}

func isMutation(query string) bool {
	return strings.HasPrefix(strings.TrimSpace(query), "mutation")
}

// Shop is what CheckConnection reports about the store.
type Shop struct {
	Name     string
	Domain   string
	PlanName string
	// DevStore is true when Shopify reports a development store.
	DevStore bool
}

// GetShop reads the store's name and plan.
func (c *Client) GetShop(ctx context.Context) (Shop, error) {
	var out struct {
		Shop struct {
			Name            string `json:"name"`
			MyshopifyDomain string `json:"myshopifyDomain"`
			Plan            struct {
				PublicDisplayName  string `json:"publicDisplayName"`
				PartnerDevelopment bool   `json:"partnerDevelopment"`
			} `json:"plan"`
		} `json:"shop"`
	}
	err := c.Query(ctx, `query { shop { name myshopifyDomain plan { publicDisplayName partnerDevelopment } } }`, nil, &out)
	if err != nil {
		return Shop{}, err
	}
	return Shop{Name: out.Shop.Name, Domain: out.Shop.MyshopifyDomain, PlanName: out.Shop.Plan.PublicDisplayName,
		DevStore: out.Shop.Plan.PartnerDevelopment}, nil
}

// IsDevStore reports whether the store is a development store. The answer
// is cached for the lifetime of the client.
func (c *Client) IsDevStore(ctx context.Context) (bool, error) {
	c.mu.Lock()
	cached := c.devStore
	c.mu.Unlock()
	if cached != nil {
		return *cached, nil
	}
	shop, err := c.GetShop(ctx)
	if err != nil {
		return false, err
	}
	// The domain must also be the one configured: a redirect or proxy must
	// not lead somewhere else.
	dev := shop.DevStore && strings.EqualFold(shop.Domain, c.cfg.StoreDomain)
	c.mu.Lock()
	c.devStore = &dev
	c.mu.Unlock()
	return dev, nil
}

// do sends one GraphQL request and retries when Shopify throttles it or
// fails temporarily.
func (c *Client) do(ctx context.Context, query string, variables map[string]any, out any) error {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		wait, err := c.once(ctx, payload, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if wait < 0 {
			return err // not worth retrying
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return fmt.Errorf("shopify: giving up after %d attempts: %w", maxAttempts, lastErr)
}

// once sends the request. On failure it returns how long to wait before a
// retry, or a negative duration when a retry is pointless.
func (c *Client) once(ctx context.Context, payload []byte, out any) (time.Duration, error) {
	const noRetry = -1
	token, err := c.accessToken(ctx)
	if err != nil {
		return noRetry, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/admin/api/"+APIVersion+"/graphql.json", bytes.NewReader(payload))
	if err != nil {
		return noRetry, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Shopify-Access-Token", token)
	res, err := c.http.Do(req)
	if err != nil {
		return time.Second, fmt.Errorf("shopify: request failed: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))

	switch {
	case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500:
		wait := 2 * time.Second
		if s := res.Header.Get("Retry-After"); s != "" {
			if d, err := time.ParseDuration(s + "s"); err == nil {
				wait = d
			}
		}
		return wait, fmt.Errorf("shopify: status %d", res.StatusCode)
	case res.StatusCode == http.StatusUnauthorized:
		// The token was revoked or expired early: get a new one once.
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()
		return 0, errors.New("shopify: access token was rejected")
	case res.StatusCode != http.StatusOK:
		return noRetry, fmt.Errorf("shopify: status %d: %s", res.StatusCode, truncate(string(body), 300))
	}

	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message    string `json:"message"`
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
		Extensions struct {
			Cost struct {
				RequestedQueryCost float64 `json:"requestedQueryCost"`
				ThrottleStatus     struct {
					CurrentlyAvailable float64 `json:"currentlyAvailable"`
					RestoreRate        float64 `json:"restoreRate"`
				} `json:"throttleStatus"`
			} `json:"cost"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return noRetry, fmt.Errorf("shopify: unreadable response: %w", err)
	}
	if len(envelope.Errors) > 0 {
		e := &GraphQLError{}
		throttled := false
		for _, ge := range envelope.Errors {
			e.Messages = append(e.Messages, ge.Message)
			throttled = throttled || ge.Extensions.Code == "THROTTLED"
		}
		if throttled {
			// Wait until the bucket holds enough points for this query.
			cost, status := envelope.Extensions.Cost, envelope.Extensions.Cost.ThrottleStatus
			wait := 2 * time.Second
			if status.RestoreRate > 0 && cost.RequestedQueryCost > status.CurrentlyAvailable {
				wait = time.Duration((cost.RequestedQueryCost-status.CurrentlyAvailable)/status.RestoreRate*float64(time.Second)) + 250*time.Millisecond
			}
			return wait, e
		}
		return noRetry, e
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return noRetry, fmt.Errorf("shopify: unexpected response shape: %w", err)
		}
	}
	return 0, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
