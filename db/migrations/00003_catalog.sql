-- +goose Up

-- Products remember which model they were cut from and how
-- (docs/decisions/0021): split_by lists the axes, split_values the values
-- this product was separated by.
ALTER TABLE product
    ADD COLUMN model_key    text  NOT NULL DEFAULT '',
    ADD COLUMN split_by     jsonb NOT NULL DEFAULT '[]',
    ADD COLUMN split_values jsonb NOT NULL DEFAULT '[]';
CREATE UNIQUE INDEX product_model_split_idx ON product (model_key, split_values) WHERE model_key <> '';

-- Own article numbers (docs/decisions/0023).
CREATE SEQUENCE variant_sku_seq START 100001;

-- +goose Down
DROP SEQUENCE variant_sku_seq;
DROP INDEX product_model_split_idx;
ALTER TABLE product DROP COLUMN model_key, DROP COLUMN split_by, DROP COLUMN split_values;
