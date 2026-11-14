-- +goose Up

-- Sources and imports -------------------------------------------------------

CREATE TABLE source (
    id                   text PRIMARY KEY,              -- e.g. 'hartje'
    name                 text NOT NULL,
    connector_kind       text NOT NULL,                 -- e.g. 'csvfile'
    connector_config     jsonb NOT NULL DEFAULT '{}',
    availability_mapping jsonb NOT NULL DEFAULT '[]',   -- ordered availability.MappingRule list
    created_at           timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE import_run (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id    text NOT NULL REFERENCES source(id),
    status       text NOT NULL CHECK (status IN ('running', 'done', 'failed', 'held')),
    file_path    text,                                  -- unchanged original delivery on disk
    file_sha256  text,
    record_count integer NOT NULL DEFAULT 0,
    issue_count  integer NOT NULL DEFAULT 0,
    summary      jsonb NOT NULL DEFAULT '{}',           -- e.g. collected quantity changes
    error        text,
    started_at   timestamptz NOT NULL DEFAULT now(),
    finished_at  timestamptz
);
CREATE INDEX import_run_source_idx ON import_run (source_id, started_at DESC);

-- One row per distinct content of one article. A new row is written only
-- when the content hash changes; rows are never updated or deleted.
CREATE TABLE raw_record (
    id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id           text NOT NULL REFERENCES source(id),
    key                 text NOT NULL,                  -- supplier article number
    content_hash        text NOT NULL,
    fields              jsonb NOT NULL,
    first_import_run_id bigint NOT NULL REFERENCES import_run(id),
    created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX raw_record_key_idx ON raw_record (source_id, key, id DESC);

-- Normalized supplier view ---------------------------------------------------

CREATE TABLE supplier_item (
    id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id          text NOT NULL REFERENCES source(id),
    sku                text NOT NULL,
    ean                text NOT NULL DEFAULT '',
    mpn                text NOT NULL DEFAULT '',
    brand              text NOT NULL DEFAULT '',
    name               text NOT NULL DEFAULT '',
    model_year         integer,
    category           text NOT NULL DEFAULT '',
    color              text NOT NULL DEFAULT '',
    frame_size         text NOT NULL DEFAULT '',
    description        text NOT NULL DEFAULT '',
    image_urls         jsonb NOT NULL DEFAULT '[]',
    cost_cents         bigint,
    rrp_cents          bigint,
    attributes         jsonb NOT NULL DEFAULT '{}',
    availability       jsonb NOT NULL DEFAULT '{}',     -- availability.Availability
    availability_status text NOT NULL DEFAULT 'unknown'
        CHECK (availability_status IN ('in_stock', 'orderable', 'preorder', 'on_request', 'unavailable', 'discontinued', 'unknown')),
    active             boolean NOT NULL DEFAULT true,   -- false once missing from the feed
    raw_record_id      bigint REFERENCES raw_record(id),
    last_import_run_id bigint REFERENCES import_run(id),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_id, sku)
);
CREATE INDEX supplier_item_ean_idx ON supplier_item (ean) WHERE ean <> '';
CREATE INDEX supplier_item_brand_idx ON supplier_item (brand);

-- Own catalog ----------------------------------------------------------------

CREATE TABLE product (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    brand            text NOT NULL DEFAULT '',
    name             text NOT NULL,
    model_year       integer,
    category         text NOT NULL DEFAULT '',
    description_html text NOT NULL DEFAULT '',
    image_urls       jsonb NOT NULL DEFAULT '[]',
    option_names     jsonb NOT NULL DEFAULT '[]',
    predecessor_id   bigint REFERENCES product(id),     -- previous model year
    status           text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'archived')),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE variant (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id       bigint NOT NULL REFERENCES product(id),
    sku              text NOT NULL UNIQUE,              -- own SKU, sent to Shopify
    ean              text NOT NULL DEFAULT '',
    option_values    jsonb NOT NULL DEFAULT '[]',
    price_cents      bigint,
    compare_at_cents bigint,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX variant_product_idx ON variant (product_id);

CREATE TABLE variant_supplier_item (
    variant_id       bigint NOT NULL REFERENCES variant(id),
    supplier_item_id bigint NOT NULL REFERENCES supplier_item(id),
    preferred        boolean NOT NULL DEFAULT false,
    PRIMARY KEY (variant_id, supplier_item_id)
);
CREATE UNIQUE INDEX variant_one_preferred_idx ON variant_supplier_item (variant_id) WHERE preferred;

-- What may be used from a source (per brand; '' = all brands).
CREATE TABLE usage_policy (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id   text NOT NULL REFERENCES source(id),
    brand       text NOT NULL DEFAULT '',
    field_group text NOT NULL CHECK (field_group IN ('images', 'texts', 'prices', 'attributes')),
    policy      text NOT NULL CHECK (policy IN ('use', 'internal_only', 'ignore')),
    UNIQUE (source_id, brand, field_group)
);

CREATE TABLE pricing_rule (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    scope_brand    text NOT NULL DEFAULT '',
    scope_category text NOT NULL DEFAULT '',
    config         jsonb NOT NULL,                      -- pricing.Config
    UNIQUE (scope_brand, scope_category)
);

-- Rules, actions, tasks ------------------------------------------------------

CREATE TABLE rule (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name            text NOT NULL,
    enabled         boolean NOT NULL DEFAULT true,
    action_type     text NOT NULL,
    scope_source_id text NOT NULL DEFAULT '',
    scope_brand     text NOT NULL DEFAULT '',
    scope_category  text NOT NULL DEFAULT '',
    conditions      jsonb NOT NULL DEFAULT '[]',
    outcome         text NOT NULL CHECK (outcome IN ('auto', 'approval')),
    guard           boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (NOT guard OR outcome = 'approval')
);

CREATE TABLE action (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    type             text NOT NULL,
    status           text NOT NULL CHECK (status IN ('pending_approval', 'auto_applied', 'approved', 'rejected', 'superseded')),
    supplier_item_id bigint REFERENCES supplier_item(id),
    product_id       bigint REFERENCES product(id),
    variant_id       bigint REFERENCES variant(id),
    field            text NOT NULL DEFAULT '',
    old_value        text,
    new_value        text,
    facts            jsonb NOT NULL DEFAULT '{}',       -- numbers the rules were evaluated on
    rule_id          bigint REFERENCES rule(id),        -- null: no rule matched
    decision_reason  text NOT NULL,
    import_run_id    bigint REFERENCES import_run(id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    decided_at       timestamptz,
    decided_by       text,
    decision_note    text
);
CREATE INDEX action_status_idx ON action (status, created_at);
CREATE INDEX action_supplier_item_idx ON action (supplier_item_id);

CREATE TABLE task (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind             text NOT NULL,                     -- e.g. 'approve_action', 'data_issue', 'sync_failed'
    title            text NOT NULL,
    detail           text NOT NULL DEFAULT '',
    status           text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'done', 'dismissed')),
    priority         smallint NOT NULL DEFAULT 2 CHECK (priority BETWEEN 1 AND 3),  -- 1 = high
    action_id        bigint REFERENCES action(id),
    product_id       bigint REFERENCES product(id),
    supplier_item_id bigint REFERENCES supplier_item(id),
    import_run_id    bigint REFERENCES import_run(id),
    external_system  text,                              -- filled by a task adapter
    external_ref     text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    closed_at        timestamptz,
    closed_by        text,
    note             text
);
CREATE INDEX task_open_idx ON task (priority, created_at) WHERE status = 'open';
CREATE UNIQUE INDEX task_action_idx ON task (action_id) WHERE action_id IS NOT NULL;

-- Provenance -----------------------------------------------------------------

-- Append-only log of every change of every field value.
CREATE TABLE field_change (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entity_type   text NOT NULL CHECK (entity_type IN ('supplier_item', 'product', 'variant')),
    entity_id     bigint NOT NULL,
    field         text NOT NULL,
    old_value     text,
    new_value     text,
    origin        text NOT NULL CHECK (origin IN ('import', 'rule', 'user', 'system')),
    import_run_id bigint REFERENCES import_run(id),
    rule_id       bigint REFERENCES rule(id),
    action_id     bigint REFERENCES action(id),
    actor         text,                                 -- who, for origin 'user'
    reason        text,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX field_change_entity_idx ON field_change (entity_type, entity_id, field, id DESC);

-- Internal only: never part of the Shopify projection ------------------------

CREATE TABLE internal_note (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entity_type text NOT NULL CHECK (entity_type IN ('product', 'variant', 'supplier_item', 'source')),
    entity_id   text NOT NULL,
    body        text NOT NULL,
    author      text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX internal_note_entity_idx ON internal_note (entity_type, entity_id);

CREATE TABLE internal_field (
    entity_type text NOT NULL CHECK (entity_type IN ('product', 'variant', 'supplier_item', 'source')),
    entity_id   text NOT NULL,
    key         text NOT NULL,                          -- e.g. 'contact', 'order_hint', 'terms'
    value       text NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (entity_type, entity_id, key)
);

-- Shopify --------------------------------------------------------------------

CREATE TABLE shopify_listing (
    product_id        bigint PRIMARY KEY REFERENCES product(id),
    wanted            boolean NOT NULL DEFAULT false,   -- should this product be in the store?
    shopify_product_id text,
    variant_ids       jsonb NOT NULL DEFAULT '{}',      -- own SKU -> Shopify variant id
    synced_projection jsonb,                            -- last projection Shopify confirmed
    synced_hash       text,
    last_synced_at    timestamptz,
    last_error        text
);

-- +goose Down
DROP TABLE shopify_listing, internal_field, internal_note, field_change, task, action, rule,
    pricing_rule, usage_policy, variant_supplier_item, variant, product, supplier_item,
    raw_record, import_run, source;
