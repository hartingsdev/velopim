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
       p.model_key, p.split_values, p.description_html, p.image_urls, coalesce(l.wanted, false)::bool AS wanted
FROM product p
LEFT JOIN shopify_listing l ON l.product_id = p.id
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

-- name: ListProductsOfSameModel :many
-- The product and, when it was cut from a model, its sibling products.
SELECT p.id, p.brand, p.name, p.model_year, p.category, p.status, p.option_names, p.created_at,
       p.model_key, p.split_values, p.description_html, p.image_urls, coalesce(l.wanted, false)::bool AS wanted
FROM product p
LEFT JOIN shopify_listing l ON l.product_id = p.id
WHERE p.id = sqlc.arg(id)::bigint
   OR (p.model_key <> '' AND p.model_key = (SELECT m.model_key FROM product m WHERE m.id = sqlc.arg(id)::bigint))
ORDER BY p.id;

-- name: ListFieldChangesOfProducts :many
-- History of products and their variants, newest first.
SELECT fc.id, fc.entity_type, fc.entity_id, fc.field, fc.old_value, fc.new_value, fc.origin, fc.actor, fc.reason,
       fc.created_at, v.sku AS variant_sku, coalesce(v.product_id, fc.entity_id)::bigint AS product_id,
       r.name AS rule_name, src.name AS source_name
FROM field_change fc
LEFT JOIN variant v ON fc.entity_type = 'variant' AND v.id = fc.entity_id
LEFT JOIN action a ON a.id = fc.action_id
LEFT JOIN rule r ON r.id = a.rule_id
LEFT JOIN supplier_item si ON si.id = a.supplier_item_id
LEFT JOIN source src ON src.id = si.source_id
WHERE (fc.entity_type = 'product' AND fc.entity_id = ANY(sqlc.arg(product_ids)::bigint[]))
   OR (fc.entity_type = 'variant' AND v.product_id = ANY(sqlc.arg(product_ids)::bigint[]))
ORDER BY fc.id DESC
LIMIT 300;

-- name: ListProductNotes :many
SELECT id, entity_id, body, author, created_at FROM internal_note
WHERE entity_type = 'product' AND entity_id = ANY(sqlc.arg(product_ids)::text[])
ORDER BY id DESC;

-- name: InsertProductNote :one
INSERT INTO internal_note (entity_type, entity_id, body, author) VALUES ('product', $1, $2, $3) RETURNING id;

-- name: GetProductText :one
SELECT name, description_html FROM product WHERE id = $1;

-- name: SetProductText :exec
UPDATE product SET name = $2, description_html = $3, updated_at = now() WHERE id = $1;

-- name: CountUnpricedVariants :one
SELECT count(*) FROM variant WHERE product_id = $1 AND price_cents IS NULL;

-- name: GetListingWanted :one
SELECT coalesce((SELECT l.wanted FROM shopify_listing l WHERE l.product_id = $1), false)::bool AS wanted;

-- name: SetListingWanted :exec
INSERT INTO shopify_listing (product_id, wanted) VALUES ($1, $2)
ON CONFLICT (product_id) DO UPDATE SET wanted = EXCLUDED.wanted;

-- name: GetVariantProduct :one
SELECT product_id FROM variant WHERE id = $1;

-- name: ListUsagePolicies :many
SELECT id, source_id, brand, field_group, policy FROM usage_policy ORDER BY source_id, brand, field_group;

-- name: UpsertUsagePolicy :exec
INSERT INTO usage_policy (source_id, brand, field_group, policy) VALUES ($1, $2, $3, $4)
ON CONFLICT (source_id, brand, field_group) DO UPDATE SET policy = EXCLUDED.policy;

-- name: DeleteUsagePolicy :execrows
DELETE FROM usage_policy WHERE source_id = $1 AND brand = $2 AND field_group = $3;

-- name: GetSupplierItemScope :one
SELECT source_id, brand FROM supplier_item WHERE id = $1;

-- name: ListWantedProducts :many
-- Products selected for the store or already in Shopify, with what Shopify
-- last confirmed.
SELECT p.id, p.brand, p.name, p.category, p.status, p.description_html, p.image_urls, p.option_names,
       l.wanted, l.shopify_product_id, l.synced_projection, l.last_synced_at, l.last_error, l.media_ids
FROM product p JOIN shopify_listing l ON l.product_id = p.id
WHERE l.wanted OR l.shopify_product_id IS NOT NULL
ORDER BY p.brand, p.model_key, p.id;

-- name: ListVariantsForProjection :many
-- Variants with the availability of their preferred supplier article.
SELECT v.product_id, v.sku, v.ean, v.option_values, v.price_cents, v.compare_at_cents,
       s.availability, s.active AS supplier_active
FROM variant v
LEFT JOIN variant_supplier_item vs ON vs.variant_id = v.id AND vs.preferred
LEFT JOIN supplier_item s ON s.id = vs.supplier_item_id
WHERE v.product_id = ANY(sqlc.arg(product_ids)::bigint[])
ORDER BY v.product_id, v.id;

-- name: SaveListingSynced :exec
UPDATE shopify_listing SET shopify_product_id = $2, variant_ids = $3, synced_projection = $4, synced_hash = $5,
    last_synced_at = $6, last_error = $7, media_ids = $8
WHERE product_id = $1;

-- name: SaveListingError :exec
UPDATE shopify_listing SET last_error = $2 WHERE product_id = $1;

-- name: HasOpenProductTask :one
SELECT EXISTS (SELECT 1 FROM task t WHERE t.product_id = sqlc.arg(product_id)::bigint AND t.kind = sqlc.arg(kind)::text AND t.status = 'open')::bool AS open;

-- name: InsertProductTask :exec
INSERT INTO task (kind, title, detail, priority, product_id) VALUES ($1, $2, $3, $4, $5);

-- name: CloseSyncFailedTasks :exec
UPDATE task SET status = 'done', closed_at = now(), closed_by = 'system', note = 'Übertragung inzwischen erfolgreich'
WHERE product_id = $1 AND kind = 'sync_failed' AND status = 'open';

-- name: GetProductStatus :one
SELECT status FROM product WHERE id = $1;

-- name: SetProductStatus :exec
UPDATE product SET status = $2, updated_at = now() WHERE id = $1;
