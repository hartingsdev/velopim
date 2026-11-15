-- +goose Up

-- A change of the supplier's purchase price proposes the same amount on the
-- own sales price (docs/decisions/0018). base_price_cents is the sales price
-- the proposal started from, proposed_price_cents the result.
ALTER TABLE action
    ADD COLUMN base_price_cents     bigint,
    ADD COLUMN proposed_price_cents bigint;

-- +goose Down
ALTER TABLE action DROP COLUMN base_price_cents, DROP COLUMN proposed_price_cents;
