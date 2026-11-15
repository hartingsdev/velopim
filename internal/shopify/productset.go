package shopify

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Metafield that carries the delivery-time text of a variant. Provisional
// until the theme's namespace and key are known (docs/open-questions.md).
const (
	DeliveryNamespace = "velopim"
	DeliveryKey       = "delivery_time"
)

// Shopify needs at least one option; a product without options gets this one.
const (
	defaultOptionName  = "Title"
	defaultOptionValue = "Default Title"
)

// ProductSetInput builds the input of the productSet mutation from a
// projection. productSet describes the whole product: variants and files
// missing from the input are removed in Shopify.
//
// No stock quantities are managed (docs/decisions/0009). A sellable variant
// is not tracked, so it can always be bought. A variant that must not be
// sold is tracked with a quantity of zero at locationID and sells out.
//
// mediaIDs maps image addresses to the media Shopify already holds for
// them; those images are referred to by id instead of being fetched again.
func ProductSetInput(p Product, locationID string, mediaIDs map[string]string) map[string]any {
	status := "DRAFT"
	if p.Published {
		status = "ACTIVE"
	}
	options := p.Options
	if len(options) == 0 {
		options = []string{defaultOptionName}
	}
	// Every option lists its values in order of first appearance.
	optionInputs := make([]map[string]any, len(options))
	for i, name := range options {
		var values []map[string]any
		seen := map[string]bool{}
		for _, v := range p.Variants {
			value := optionValue(v, i, len(p.Options))
			if !seen[value] {
				seen[value] = true
				values = append(values, map[string]any{"name": value})
			}
		}
		optionInputs[i] = map[string]any{"name": name, "position": i + 1, "values": values}
	}

	variants := make([]map[string]any, len(p.Variants))
	for i, v := range p.Variants {
		optionValues := make([]map[string]any, len(options))
		for j, name := range options {
			optionValues[j] = map[string]any{"optionName": name, "name": optionValue(v, j, len(p.Options))}
		}
		variant := map[string]any{
			"sku":          v.SKU,
			"position":     i + 1,
			"optionValues": optionValues,
			"price":        money(v.PriceCents),
			"metafields": []map[string]any{{
				"namespace": DeliveryNamespace, "key": DeliveryKey, "type": "single_line_text_field", "value": v.DeliveryText,
			}},
			"inventoryPolicy": "DENY",
		}
		if v.CompareAtCents > 0 {
			variant["compareAtPrice"] = money(v.CompareAtCents)
		} else {
			variant["compareAtPrice"] = nil
		}
		if v.Barcode != "" {
			variant["barcodes"] = []map[string]any{{"value": v.Barcode}}
		} else {
			variant["barcodes"] = []map[string]any{}
		}
		if v.Sellable {
			variant["inventoryItem"] = map[string]any{"tracked": false}
		} else {
			variant["inventoryItem"] = map[string]any{"tracked": true}
			variant["inventoryQuantities"] = []map[string]any{{"locationId": locationID, "name": "available", "quantity": 0}}
		}
		variants[i] = variant
	}

	files := make([]map[string]any, len(p.ImageURLs))
	for i, u := range p.ImageURLs {
		if id := mediaIDs[u]; id != "" {
			files[i] = map[string]any{"id": id}
			continue
		}
		files[i] = map[string]any{"originalSource": u, "contentType": "IMAGE", "alt": p.Title}
	}

	return map[string]any{
		"title":           p.Title,
		"descriptionHtml": p.DescriptionHTML,
		"vendor":          p.Vendor,
		"productType":     p.ProductType,
		"status":          status,
		"productOptions":  optionInputs,
		"variants":        variants,
		"files":           files,
	}
}

func optionValue(v Variant, i, realOptions int) string {
	if realOptions == 0 {
		return defaultOptionValue
	}
	if i < len(v.OptionValues) && v.OptionValues[i] != "" {
		return v.OptionValues[i]
	}
	return "–"
}

func money(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

// SetResult is what Shopify reports after a product was written.
type SetResult struct {
	ProductID  string // GraphQL id
	NumericID  string // id used in admin URLs
	Handle     string
	Status     string
	PreviewURL string
	// VariantIDs maps SKU to the variant's GraphQL id.
	VariantIDs map[string]string
	// AvailableForSale maps SKU to what Shopify reports to customers.
	AvailableForSale map[string]bool
	// MediaIDs are the product's media in the order of the images sent.
	MediaIDs []string
}

// UserError is a validation error of a mutation: Shopify understood the
// request but refuses the data. Retrying does not help.
type UserError struct{ Messages []string }

func (e *UserError) Error() string { return "shopify: " + strings.Join(e.Messages, "; ") }

const productSetMutation = `mutation velopimProductSet($input: ProductSetInput!, $identifier: ProductSetIdentifiers) {
  productSet(input: $input, synchronous: true, identifier: $identifier) {
    product {
      id legacyResourceId handle status onlineStorePreviewUrl
      variants(first: 250) { nodes { id sku availableForSale } }
      media(first: 250) { nodes { id } }
    }
    userErrors { field message code }
  }
}`

// SetProduct creates the product, or replaces the product with the given
// Shopify id (empty for a new one). Calling it again with the same input
// changes nothing. mediaIDs are the images Shopify already holds, by address.
func (c *Client) SetProduct(ctx context.Context, p Product, shopifyID string, mediaIDs map[string]string) (SetResult, error) {
	locationID, err := c.locationID(ctx)
	if err != nil {
		return SetResult{}, err
	}
	variables := map[string]any{"input": ProductSetInput(p, locationID, mediaIDs)}
	if shopifyID != "" {
		variables["identifier"] = map[string]any{"id": shopifyID}
	}
	var out struct {
		ProductSet struct {
			Product *struct {
				ID                    string `json:"id"`
				LegacyResourceID      string `json:"legacyResourceId"`
				Handle                string `json:"handle"`
				Status                string `json:"status"`
				OnlineStorePreviewURL string `json:"onlineStorePreviewUrl"`
				Variants              struct {
					Nodes []struct {
						ID               string `json:"id"`
						SKU              string `json:"sku"`
						AvailableForSale bool   `json:"availableForSale"`
					} `json:"nodes"`
				} `json:"variants"`
				Media struct {
					Nodes []struct {
						ID string `json:"id"`
					} `json:"nodes"`
				} `json:"media"`
			} `json:"product"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"productSet"`
	}
	if err := c.Mutate(ctx, productSetMutation, variables, &out); err != nil {
		return SetResult{}, err
	}
	if len(out.ProductSet.UserErrors) > 0 {
		e := &UserError{}
		for _, ue := range out.ProductSet.UserErrors {
			msg := ue.Message
			if len(ue.Field) > 0 {
				msg = strings.Join(ue.Field, ".") + ": " + msg
			}
			e.Messages = append(e.Messages, msg)
		}
		return SetResult{}, e
	}
	prod := out.ProductSet.Product
	if prod == nil {
		return SetResult{}, errors.New("shopify: productSet returned no product")
	}
	res := SetResult{ProductID: prod.ID, NumericID: prod.LegacyResourceID, Handle: prod.Handle, Status: prod.Status,
		PreviewURL: prod.OnlineStorePreviewURL, VariantIDs: map[string]string{}, AvailableForSale: map[string]bool{}}
	for _, v := range prod.Variants.Nodes {
		res.VariantIDs[v.SKU] = v.ID
		res.AvailableForSale[v.SKU] = v.AvailableForSale
	}
	for _, m := range prod.Media.Nodes {
		res.MediaIDs = append(res.MediaIDs, m.ID)
	}
	// An active product is only visible to customers once it is published
	// in the online store's sales channel. If that fails the whole call
	// fails, so the sync tries again instead of recording a product nobody
	// can see as done.
	if p.Published {
		if err := c.publishToOnlineStore(ctx, prod.ID); err != nil {
			return SetResult{}, err
		}
	}
	return res, nil
}

// onlineStoreHandle is the handle of the sales channel "Onlineshop".
const onlineStoreHandle = "online_store"

// ErrNoOnlineStore means the store has no online store sales channel the
// app can see; without the right read_publications Shopify hides them all.
var ErrNoOnlineStore = errors.New("shopify: sales channel \"Online Store\" not found (does the app have the right write_publications?)")

// publishToOnlineStore makes the product visible in the online store.
// Publishing a product that is already published changes nothing. A product
// set back to draft is not unpublished: a draft is not visible anyway, and
// it reappears when it becomes active again.
func (c *Client) publishToOnlineStore(ctx context.Context, shopifyID string) error {
	publicationID, err := c.onlineStorePublicationID(ctx)
	if err != nil {
		return err
	}
	var out struct {
		PublishablePublish struct {
			UserErrors []struct {
				Message string `json:"message"`
			} `json:"userErrors"`
		} `json:"publishablePublish"`
	}
	err = c.Mutate(ctx, `mutation velopimPublish($id: ID!, $publication: ID!) {
  publishablePublish(id: $id, input: [{publicationId: $publication}]) { userErrors { field message } }
}`, map[string]any{"id": shopifyID, "publication": publicationID}, &out)
	if err != nil {
		return fmt.Errorf("publish in online store: %w", err)
	}
	if len(out.PublishablePublish.UserErrors) > 0 {
		e := &UserError{}
		for _, ue := range out.PublishablePublish.UserErrors {
			e.Messages = append(e.Messages, "publish in online store: "+ue.Message)
		}
		return e
	}
	return nil
}

// onlineStorePublicationID finds the publication of the online store
// channel; the id is remembered for the life of the client.
func (c *Client) onlineStorePublicationID(ctx context.Context) (string, error) {
	c.mu.Lock()
	cached := c.onlineStore
	c.mu.Unlock()
	if cached != "" {
		return cached, nil
	}
	var out struct {
		Publications struct {
			Nodes []struct {
				ID       string `json:"id"`
				Channels struct {
					Nodes []struct {
						Handle string `json:"handle"`
					} `json:"nodes"`
				} `json:"channels"`
			} `json:"nodes"`
		} `json:"publications"`
	}
	if err := c.Query(ctx, `query { publications(first: 50) { nodes { id channels(first: 10) { nodes { handle } } } } }`, nil, &out); err != nil {
		return "", err
	}
	for _, pub := range out.Publications.Nodes {
		for _, ch := range pub.Channels.Nodes {
			if ch.Handle == onlineStoreHandle {
				c.mu.Lock()
				c.onlineStore = pub.ID
				c.mu.Unlock()
				return pub.ID, nil
			}
		}
	}
	return "", ErrNoOnlineStore
}

// PublishedInOnlineStore reports whether customers can see the product in
// the online store's sales channel.
func (c *Client) PublishedInOnlineStore(ctx context.Context, shopifyID string) (bool, error) {
	publicationID, err := c.onlineStorePublicationID(ctx)
	if err != nil {
		return false, err
	}
	var out struct {
		Product *struct {
			Published bool `json:"publishedOnPublication"`
		} `json:"product"`
	}
	err = c.Query(ctx, `query($id: ID!, $publication: ID!) { product(id: $id) { publishedOnPublication(publicationId: $publication) } }`,
		map[string]any{"id": shopifyID, "publication": publicationID}, &out)
	if err != nil {
		return false, err
	}
	if out.Product == nil {
		return false, errors.New("shopify: product not found")
	}
	return out.Product.Published, nil
}

// DeleteProduct removes a product from Shopify.
func (c *Client) DeleteProduct(ctx context.Context, shopifyID string) error {
	var out struct {
		ProductDelete struct {
			UserErrors []struct {
				Message string `json:"message"`
			} `json:"userErrors"`
		} `json:"productDelete"`
	}
	err := c.Mutate(ctx, `mutation velopimProductDelete($id: ID!) { productDelete(input: {id: $id}, synchronous: true) { deletedProductId userErrors { field message } } }`,
		map[string]any{"id": shopifyID}, &out)
	if err != nil {
		return err
	}
	if len(out.ProductDelete.UserErrors) > 0 {
		e := &UserError{}
		for _, ue := range out.ProductDelete.UserErrors {
			e.Messages = append(e.Messages, ue.Message)
		}
		return e
	}
	return nil
}

// locationID returns the store's first location; untracked stock of
// unsellable variants is set to zero there.
func (c *Client) locationID(ctx context.Context) (string, error) {
	c.mu.Lock()
	cached := c.location
	c.mu.Unlock()
	if cached != "" {
		return cached, nil
	}
	var out struct {
		Locations struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		} `json:"locations"`
	}
	if err := c.Query(ctx, `query { locations(first: 1) { nodes { id } } }`, nil, &out); err != nil {
		return "", err
	}
	if len(out.Locations.Nodes) == 0 {
		return "", errors.New("shopify: the store has no location")
	}
	c.mu.Lock()
	c.location = out.Locations.Nodes[0].ID
	c.mu.Unlock()
	return out.Locations.Nodes[0].ID, nil
}

// AdminURL is the product's page in the Shopify admin.
func (c *Client) AdminURL(numericID string) string {
	store := strings.TrimSuffix(c.cfg.StoreDomain, ".myshopify.com")
	return "https://admin.shopify.com/store/" + store + "/products/" + numericID
}

// MediaState is the processing state of one image of a product.
type MediaState struct {
	ID     string
	Status string // UPLOADED, PROCESSING, READY or FAILED
	URL    string // where Shopify serves the image, once ready
	Errors []string
}

// ProductMedia reads the images Shopify holds for a product. Shopify fetches
// images after the sync, so their state is only known later.
func (c *Client) ProductMedia(ctx context.Context, shopifyID string) ([]MediaState, error) {
	var out struct {
		Product *struct {
			Media struct {
				Nodes []struct {
					ID          string `json:"id"`
					Status      string `json:"status"`
					MediaErrors []struct {
						Message string `json:"message"`
					} `json:"mediaErrors"`
					Image *struct {
						URL string `json:"url"`
					} `json:"image"`
				} `json:"nodes"`
			} `json:"media"`
		} `json:"product"`
	}
	err := c.Query(ctx, `query($id: ID!) { product(id: $id) { media(first: 50) { nodes {
		id status mediaErrors { message } ... on MediaImage { image { url } } } } } }`, map[string]any{"id": shopifyID}, &out)
	if err != nil {
		return nil, err
	}
	if out.Product == nil {
		return nil, errors.New("shopify: product not found")
	}
	states := make([]MediaState, 0, len(out.Product.Media.Nodes))
	for _, n := range out.Product.Media.Nodes {
		s := MediaState{ID: n.ID, Status: n.Status}
		if n.Image != nil {
			s.URL = n.Image.URL
		}
		for _, e := range n.MediaErrors {
			s.Errors = append(s.Errors, e.Message)
		}
		states = append(states, s)
	}
	return states, nil
}

// WaitForMedia waits until Shopify has finished fetching a product's images
// or the time is up, and returns their states.
func (c *Client) WaitForMedia(ctx context.Context, shopifyID string, limit time.Duration) ([]MediaState, error) {
	deadline := time.Now().Add(limit)
	for {
		states, err := c.ProductMedia(ctx, shopifyID)
		if err != nil {
			return nil, err
		}
		pending := false
		for _, s := range states {
			pending = pending || s.Status == "UPLOADED" || s.Status == "PROCESSING"
		}
		if !pending || time.Now().After(deadline) {
			return states, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
