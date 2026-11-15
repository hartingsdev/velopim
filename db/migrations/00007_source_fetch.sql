-- +goose Up

-- velopim can fetch a supplier's file itself (docs/decisions/0030).
-- fetch_config says where and how often; the last attempt is kept for the
-- interface.
ALTER TABLE source
    ADD COLUMN fetch_config     jsonb NOT NULL DEFAULT '{}',
    ADD COLUMN last_fetch_at    timestamptz,
    ADD COLUMN last_fetch_error text;

-- Tasks about a source as a whole, e.g. a failed fetch.
ALTER TABLE task ADD COLUMN source_id text REFERENCES source(id);

-- +goose Down
ALTER TABLE task DROP COLUMN source_id;
ALTER TABLE source DROP COLUMN fetch_config, DROP COLUMN last_fetch_at, DROP COLUMN last_fetch_error;
