-- +goose Up

-- Shopify media id per image address, so that a later sync refers to the
-- image Shopify already holds instead of fetching it again.
ALTER TABLE shopify_listing ADD COLUMN media_ids jsonb NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE shopify_listing DROP COLUMN media_ids;
