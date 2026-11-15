# 5. Lieferantenartikel getrennt von Produkt und Variante

**Status:** Angenommen

## Kontext
Ein Rad kann später von mehreren Lieferanten kommen. Lieferantendaten und eigene Produktdaten ändern sich unabhängig voneinander.

## Entscheidung
`supplier_item` ist die normalisierte Sicht einer Quelle. `product` und `variant` sind der eigene Katalog. `variant_supplier_item` verbindet beide n:m, mit einem bevorzugten Lieferanten je Variante.

## Alternativen
- **Ein Modell für beides:** einfacher im ersten Schritt, aber der zweite Lieferant erzwingt einen Umbau des Schemas.

## Folgen
Zwei Ebenen, die gepflegt werden müssen. Dafür bleibt der eigene Katalog stabil, wenn eine Quelle wechselt.
