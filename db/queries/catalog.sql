-- name: ListActionSupplierItems :many
SELECT s.id, s.source_id, s.sku, s.ean, s.brand, s.name, s.model_year, s.category, s.color, s.frame_size,
       s.description, s.image_urls, s.cost_cents, s.rrp_cents, s.attributes, s.active
FROM action_item ai JOIN supplier_item s ON s.id = ai.supplier_item_id
WHERE ai.action_id = $1
ORDER BY s.sku;

-- name: ListProductsOfModel :many
SELECT id, split_by, split_values FROM product WHERE model_key = $1 ORDER BY id;

-- name: InsertProduct :one
INSERT INTO product (brand, name, model_year, category, description_html, image_urls, option_names,
    model_key, split_by, split_values)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id;

-- name: SupplierItemHasVariant :one
SELECT EXISTS (SELECT 1 FROM variant_supplier_item WHERE supplier_item_id = $1);

-- name: NextVariantNumber :one
SELECT nextval('variant_sku_seq');

-- name: InsertVariant :one
INSERT INTO variant (product_id, sku, ean, option_values, price_cents, compare_at_cents)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: LinkVariantSupplierItem :exec
INSERT INTO variant_supplier_item (variant_id, supplier_item_id, preferred) VALUES ($1, $2, true);

-- name: InsertCatalogFieldChange :exec
INSERT INTO field_change (entity_type, entity_id, field, old_value, new_value, origin, action_id, actor, reason)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListProducts :many
-- status '' means all; unpriced limits to products with a variant without
-- sales price; q matches brand, name, own and supplier article numbers, EANs.
SELECT p.id, p.brand, p.name, p.model_year, p.category, p.status, p.option_names, p.created_at,
       p.model_key, p.split_values
FROM product p
WHERE (sqlc.arg(status)::text = '' OR p.status = sqlc.arg(status)::text)
  AND (NOT sqlc.arg(unpriced)::bool
       OR EXISTS (SELECT 1 FROM variant v WHERE v.product_id = p.id AND v.price_cents IS NULL))
  AND (sqlc.arg(q)::text = ''
       OR strpos(lower(p.brand || ' ' || p.name), lower(sqlc.arg(q)::text)) > 0
       OR EXISTS (SELECT 1 FROM variant v
                  LEFT JOIN variant_supplier_item vs ON vs.variant_id = v.id
                  LEFT JOIN supplier_item s ON s.id = vs.supplier_item_id
                  WHERE v.product_id = p.id
                    AND strpos(lower(v.sku || ' ' || v.ean || ' ' || coalesce(s.sku, '')), lower(sqlc.arg(q)::text)) > 0))
ORDER BY p.brand, p.model_key, p.id
LIMIT sqlc.arg(max_rows);

-- name: CountProducts :one
SELECT count(*) AS total,
       count(*) FILTER (WHERE EXISTS (SELECT 1 FROM variant v WHERE v.product_id = p.id AND v.price_cents IS NULL)) AS unpriced
FROM product p;

-- name: ListVariantsOfProducts :many
SELECT v.id, v.product_id, v.sku, v.ean, v.option_values, v.price_cents, v.compare_at_cents,
       s.sku AS supplier_sku, s.source_id, s.cost_cents, s.rrp_cents, s.availability_status
FROM variant v
LEFT JOIN variant_supplier_item vs ON vs.variant_id = v.id AND vs.preferred
LEFT JOIN supplier_item s ON s.id = vs.supplier_item_id
WHERE v.product_id = ANY(sqlc.arg(product_ids)::bigint[])
ORDER BY v.product_id, v.option_values::text, v.sku;

-- name: ListAllPricingRules :many
SELECT id, scope_brand, scope_category, config FROM pricing_rule ORDER BY scope_brand, scope_category;

-- name: UpsertPricingRule :one
INSERT INTO pricing_rule (scope_brand, scope_category, config) VALUES ($1, $2, $3)
ON CONFLICT (scope_brand, scope_category) DO UPDATE SET config = EXCLUDED.config
RETURNING id;

-- name: DeletePricingRule :execrows
DELETE FROM pricing_rule WHERE id = $1;

-- name: ListModelSplits :many
SELECT DISTINCT model_key, split_by FROM product WHERE model_key = ANY(sqlc.arg(model_keys)::text[]);

-- name: GetPricedVariantOfSupplierItem :one
SELECT v.id, v.price_cents FROM variant v
JOIN variant_supplier_item vs ON vs.variant_id = v.id AND vs.preferred
WHERE vs.supplier_item_id = $1 AND v.price_cents IS NOT NULL;

-- name: ShiftVariantPrice :one
-- Changes the sales price by an amount. The compare-at price is dropped once
-- the sales price reaches it.
UPDATE variant SET
    price_cents = price_cents + sqlc.arg(delta_cents)::bigint,
    compare_at_cents = CASE WHEN compare_at_cents <= price_cents + sqlc.arg(delta_cents)::bigint THEN NULL ELSE compare_at_cents END,
    updated_at = now()
WHERE id = $1 AND price_cents IS NOT NULL AND price_cents + sqlc.arg(delta_cents)::bigint > 0
RETURNING price_cents;

-- name: GetVariantPrice :one
SELECT price_cents FROM variant WHERE id = $1;

-- name: SetVariantPrice :exec
UPDATE variant SET
    price_cents = sqlc.arg(price_cents)::bigint,
    compare_at_cents = CASE WHEN compare_at_cents <= sqlc.arg(price_cents)::bigint THEN NULL ELSE compare_at_cents END,
    updated_at = now()
WHERE id = $1;

-- name: SetActionProposedPrice :exec
UPDATE action SET proposed_price_cents = $2 WHERE id = $1;

-- name: GetVariantOfSupplierItem :one
-- The variant an article is the preferred supplier of.
SELECT v.id, v.product_id, v.ean, v.price_cents, v.compare_at_cents, p.description_html
FROM variant v
JOIN variant_supplier_item vs ON vs.variant_id = v.id AND vs.preferred
JOIN product p ON p.id = v.product_id
WHERE vs.supplier_item_id = $1;

-- name: SetProductDescription :exec
UPDATE product SET description_html = $2, updated_at = now() WHERE id = $1;

-- name: GetProductImages :one
SELECT image_urls FROM product WHERE id = $1;

-- name: ListSupplierImagesOfProduct :many
-- Image lists of the preferred supplier articles behind a product's variants.
SELECT s.image_urls
FROM variant v
JOIN variant_supplier_item vs ON vs.variant_id = v.id AND vs.preferred
JOIN supplier_item s ON s.id = vs.supplier_item_id
WHERE v.product_id = $1
ORDER BY v.id;

-- name: SetProductImages :exec
UPDATE product SET image_urls = $2, updated_at = now() WHERE id = $1;

-- name: SetVariantCompareAt :exec
UPDATE variant SET compare_at_cents = $2, updated_at = now() WHERE id = $1;

-- name: SetVariantEAN :exec
UPDATE variant SET ean = $2, updated_at = now() WHERE id = $1;
