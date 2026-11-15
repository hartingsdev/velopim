package shopify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeShopify answers token requests and hands GraphQL requests to handle.
func fakeShopify(t *testing.T, cfg Config, handle func(query string, w http.ResponseWriter)) (*Client, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	var tokens, queries atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin/oauth/access_token" {
			tokens.Add(1)
			_ = r.ParseForm()
			if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_secret") != "secret" {
				http.Error(w, `{"error":"invalid_client"}`, http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"tok","scope":"write_products,read_locations","expires_in":86399}`)
			return
		}
		if r.URL.Path != "/admin/api/"+APIVersion+"/graphql.json" || r.Header.Get("X-Shopify-Access-Token") != "tok" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		queries.Add(1)
		var body struct{ Query string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		handle(body.Query, w)
	}))
	t.Cleanup(srv.Close)
	cfg.StoreDomain, cfg.ClientID, cfg.ClientSecret = "test-dev.myshopify.com", "id", "secret"
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	c.base = srv.URL
	return c, &tokens, &queries
}

func shopAnswer(dev bool, domain string) string {
	b, _ := json.Marshal(map[string]any{"data": map[string]any{"shop": map[string]any{
		"name": "Test", "myshopifyDomain": domain, "plan": map[string]any{"publicDisplayName": "Basic", "partnerDevelopment": dev}}}})
	return string(b)
}

func TestNewClientValidates(t *testing.T) {
	for _, domain := range []string{"", "example.com", "shop.myshopify.com.evil.test", "https://shop.myshopify.com", "shop.myshopify.com/x"} {
		if _, err := NewClient(Config{StoreDomain: domain, ClientID: "a", ClientSecret: "b"}); err == nil {
			t.Errorf("domain %q was accepted", domain)
		}
	}
	if _, err := NewClient(Config{StoreDomain: "shop.myshopify.com"}); err == nil {
		t.Error("missing credentials were accepted")
	}
	if _, err := NewClient(Config{StoreDomain: " Shop-1.MyShopify.com ", ClientID: "a", ClientSecret: "b"}); err != nil {
		t.Errorf("valid domain refused: %v", err)
	}
}

func TestQueryReusesTheToken(t *testing.T) {
	c, tokens, queries := fakeShopify(t, Config{}, func(_ string, w http.ResponseWriter) {
		_, _ = io.WriteString(w, shopAnswer(true, "test-dev.myshopify.com"))
	})
	for range 3 {
		shop, err := c.GetShop(context.Background())
		if err != nil || !shop.DevStore || shop.PlanName != "Basic" {
			t.Fatalf("GetShop: %+v %v", shop, err)
		}
	}
	if tokens.Load() != 1 || queries.Load() != 3 {
		t.Errorf("token requests = %d, queries = %d; want 1 and 3", tokens.Load(), queries.Load())
	}
	if err := c.Query(context.Background(), "mutation { x }", nil, nil); err == nil {
		t.Error("Query accepted a mutation")
	}
}

func TestMutationsAreGuarded(t *testing.T) {
	ctx := context.Background()
	mutation := `mutation { productDelete(input: {id: "x"}) { deletedProductId } }`

	// Writing switched off: nothing is sent at all.
	c, tokens, queries := fakeShopify(t, Config{WriteEnabled: false}, func(string, http.ResponseWriter) {
		t.Error("request sent although writing is disabled")
	})
	if err := c.Mutate(ctx, mutation, nil, nil); !errors.Is(err, ErrWritesDisabled) {
		t.Errorf("got %v, want ErrWritesDisabled", err)
	}
	if tokens.Load() != 0 || queries.Load() != 0 {
		t.Error("disabled client talked to the server")
	}

	// Not a development store, or another domain than configured: only the
	// shop is read, the mutation is never sent.
	for name, answer := range map[string]string{
		"live store":   shopAnswer(false, "test-dev.myshopify.com"),
		"other domain": shopAnswer(true, "somewhere-else.myshopify.com"),
	} {
		c, _, _ := fakeShopify(t, Config{WriteEnabled: true}, func(query string, w http.ResponseWriter) {
			if strings.HasPrefix(query, "mutation") {
				t.Errorf("%s: mutation was sent", name)
			}
			_, _ = io.WriteString(w, answer)
		})
		if err := c.Mutate(ctx, mutation, nil, nil); !errors.Is(err, ErrNotDevStore) {
			t.Errorf("%s: got %v, want ErrNotDevStore", name, err)
		}
	}

	// Development store with writing enabled: the mutation goes through.
	sent := false
	c, _, _ = fakeShopify(t, Config{WriteEnabled: true}, func(query string, w http.ResponseWriter) {
		if strings.HasPrefix(query, "mutation") {
			sent = true
			_, _ = io.WriteString(w, `{"data":{"productDelete":{"deletedProductId":"x"}}}`)
			return
		}
		_, _ = io.WriteString(w, shopAnswer(true, "test-dev.myshopify.com"))
	})
	if err := c.Mutate(ctx, mutation, nil, nil); err != nil || !sent {
		t.Errorf("allowed mutation: sent=%v err=%v", sent, err)
	}
}

func TestThrottledRequestIsRetried(t *testing.T) {
	var calls atomic.Int32
	c, _, _ := fakeShopify(t, Config{}, func(_ string, w http.ResponseWriter) {
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(w, `{"errors":[{"message":"Throttled","extensions":{"code":"THROTTLED"}}],
				"extensions":{"cost":{"requestedQueryCost":10,"throttleStatus":{"currentlyAvailable":9,"restoreRate":1000}}}}`)
			return
		}
		_, _ = io.WriteString(w, shopAnswer(true, "test-dev.myshopify.com"))
	})
	if _, err := c.GetShop(context.Background()); err != nil {
		t.Fatalf("throttled request was not retried: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

func TestGraphQLErrorIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	c, _, _ := fakeShopify(t, Config{}, func(_ string, w http.ResponseWriter) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"errors":[{"message":"Field 'nope' doesn't exist"}]}`)
	})
	_, err := c.GetShop(context.Background())
	var ge *GraphQLError
	if !errors.As(err, &ge) || calls.Load() != 1 {
		t.Errorf("err = %v after %d calls; want a GraphQLError after 1", err, calls.Load())
	}
}

func TestSetProductPublishesActiveProducts(t *testing.T) {
	ctx := context.Background()
	product := `{"data":{"productSet":{"product":{"id":"gid://shopify/Product/1","legacyResourceId":"1","handle":"h","status":"ACTIVE",
		"variants":{"nodes":[]},"media":{"nodes":[]}},"userErrors":[]}}}`
	publications := `{"data":{"publications":{"nodes":[
		{"id":"gid://shopify/Publication/7","channels":{"nodes":[{"handle":"pos"}]}},
		{"id":"gid://shopify/Publication/8","channels":{"nodes":[{"handle":"online_store"}]}}]}}}`

	for _, tc := range []struct {
		name         string
		published    bool
		publications string
		publishReply string
		wantPublish  int
		wantErr      bool
	}{
		{name: "draft is not published", published: false, publications: publications},
		{name: "active is published", published: true, publications: publications,
			publishReply: `{"data":{"publishablePublish":{"userErrors":[]}}}`, wantPublish: 1},
		{name: "refused publishing fails the call", published: true, publications: publications,
			publishReply: `{"data":{"publishablePublish":{"userErrors":[{"message":"nope"}]}}}`, wantPublish: 1, wantErr: true},
		{name: "no online store channel", published: true, publications: `{"data":{"publications":{"nodes":[]}}}`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			publishes := 0
			c, _, _ := fakeShopify(t, Config{WriteEnabled: true}, func(query string, w http.ResponseWriter) {
				switch {
				case strings.Contains(query, "publishablePublish"):
					publishes++
					if !strings.Contains(query, "publicationId: $publication") {
						t.Errorf("publish does not name the publication: %s", query)
					}
					_, _ = io.WriteString(w, tc.publishReply)
				case strings.Contains(query, "productSet"):
					_, _ = io.WriteString(w, product)
				case strings.Contains(query, "publications"):
					_, _ = io.WriteString(w, tc.publications)
				case strings.Contains(query, "locations"):
					_, _ = io.WriteString(w, `{"data":{"locations":{"nodes":[{"id":"gid://shopify/Location/1"}]}}}`)
				default:
					_, _ = io.WriteString(w, shopAnswer(true, "test-dev.myshopify.com"))
				}
			})
			p := Product{Title: "Rad", Published: tc.published, Variants: []Variant{{SKU: "1", PriceCents: 100, Sellable: true}}}
			_, err := c.SetProduct(ctx, p, "", nil)
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, want error: %v", err, tc.wantErr)
			}
			if publishes != tc.wantPublish {
				t.Errorf("publish calls = %d, want %d", publishes, tc.wantPublish)
			}
		})
	}
}
