-- +goose Up

-- Own article numbers are plain short numbers without a prefix
-- (docs/decisions/0023). Nothing has been synced to Shopify yet, so existing
-- variants are renumbered.
ALTER SEQUENCE variant_sku_seq RESTART WITH 10001;
UPDATE variant v SET sku = n.sku
FROM (SELECT o.id, nextval('variant_sku_seq')::text AS sku FROM (SELECT id FROM variant ORDER BY id) o) n
WHERE n.id = v.id;

-- +goose Down
-- The earlier numbers are not restored.
