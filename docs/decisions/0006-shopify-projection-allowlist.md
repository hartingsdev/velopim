# 6. Shopify-Projektion als Positivliste

**Status:** Angenommen

## Kontext
Interne Notizen, Einkaufspreise und Lieferanteninterna dürfen nie zu Shopify gelangen.

## Entscheidung
Der Sync liest ausschließlich aus einer Projektion (`shopify.Product`, `shopify.Variant`). Dort stehen nur Felder, die ausdrücklich für Shopify bestimmt sind. Ein Test hält die Feldliste fest und schlägt fehl, wenn ein Feld dazukommt.

## Alternativen
- **Sperrliste interner Felder:** ein neues internes Feld wäre ohne weiteres Zutun öffentlich.

## Folgen
Jedes neue Feld für Shopify ist eine bewusste Änderung an zwei Stellen (Projektion und Test).
