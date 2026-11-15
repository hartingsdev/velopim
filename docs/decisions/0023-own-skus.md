# 23. Eigene Artikelnummern für Varianten

**Status:** Angenommen (Vorgabe des Betreibers); Format vorläufig

## Kontext
Jede Variante braucht eine SKU, die zu Shopify geht. Zur Wahl standen die Artikelnummer des Lieferanten oder ein eigenes Schema.

## Entscheidung
Varianten bekommen eigene, fortlaufende Artikelnummern im Format `VP-100001` aus einer Datenbanksequenz. Die Artikelnummer des Lieferanten bleibt am `supplier_item` und ist über `variant_supplier_item` verknüpft.

## Alternativen
- **Lieferantennummer übernehmen:** kein Aufwand, aber die SKU ändert sich beim Wechsel des Lieferanten und ist bei zwei Lieferanten nicht eindeutig.

## Folgen
Die SKU bleibt stabil, auch wenn der bevorzugte Lieferant wechselt. Das Format (`VP-` plus Nummer) hat der Betreiber nicht festgelegt; es lässt sich ändern, solange nichts zu Shopify synchronisiert ist. Für Bestellungen beim Lieferanten muss die Oberfläche dessen Artikelnummer neben der eigenen zeigen.
