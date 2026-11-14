-- Example source for local development, matching testdata/example_supplier.csv.
-- Invented layout, not the Hartje format. Load with:
--   psql "$DATABASE_URL" -f testdata/example_source.sql
-- There are deliberately no rules: every change needs approval.
INSERT INTO source (id, name, connector_kind, connector_config, availability_mapping) VALUES (
  'example', 'Beispiel-Lieferant', 'csvfile',
  '{
    "delimiter": ";",
    "decimal_comma": true,
    "columns": {
      "sku": "Artikelnr", "ean": "EAN", "brand": "Marke", "name": "Bezeichnung", "model_year": "Modelljahr",
      "color": "Farbe", "frame_size": "Rahmenhoehe", "cost": "EK", "rrp": "UVP", "images": "Bilder"
    },
    "attribute_columns": ["Antrieb"]
  }',
  '[
    {"name": "in stock", "when": [{"field": "Bestand", "op": "gt", "value": "0"}], "status": "in_stock", "quantity_field": "Bestand"},
    {"name": "with date", "when": [{"field": "Liefertermin", "op": "not_empty"}], "status": "preorder", "date_field": "Liefertermin", "date_layout": "02.01.2006"},
    {"name": "sold out", "when": [{"field": "Bestand", "op": "lte", "value": "0"}], "status": "unavailable"}
  ]'
) ON CONFLICT (id) DO NOTHING;
