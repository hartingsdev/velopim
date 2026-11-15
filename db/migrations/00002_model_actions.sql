-- +goose Up

-- A new model is one action covering all its new supplier items, instead of
-- one action per item. model_key identifies the model within its source;
-- action_item lists the items the action covers.
ALTER TABLE action ADD COLUMN model_key text NOT NULL DEFAULT '';
CREATE INDEX action_model_key_idx ON action (model_key) WHERE model_key <> '';

CREATE TABLE action_item (
    action_id        bigint NOT NULL REFERENCES action(id),
    supplier_item_id bigint NOT NULL REFERENCES supplier_item(id),
    PRIMARY KEY (action_id, supplier_item_id)
);
CREATE INDEX action_item_item_idx ON action_item (supplier_item_id);

-- +goose Down
DROP TABLE action_item;
ALTER TABLE action DROP COLUMN model_key;
