# 23. Eigene Artikelnummern für Varianten

**Status:** Angenommen (Vorgabe des Betreibers), umgesetzt

## Kontext
Jede Variante braucht eine SKU, die zu Shopify geht. Zur Wahl standen die Artikelnummer des Lieferanten oder ein eigenes Schema.

## Entscheidung
Varianten bekommen eigene, fortlaufende Artikelnummern aus einer Datenbanksequenz: reine Zahlen ohne Präfix, fünfstellig ab `10001`. Vorgabe des Betreibers: für Kunden einfach und nicht zu lang. Die Artikelnummer des Lieferanten bleibt am `supplier_item` und ist über `variant_supplier_item` verknüpft.

## Alternativen
- **Lieferantennummer übernehmen:** kein Aufwand, aber die SKU ändert sich beim Wechsel des Lieferanten und ist bei zwei Lieferanten nicht eindeutig.

## Folgen
Die SKU bleibt stabil, auch wenn der bevorzugte Lieferant wechselt. Ein zuerst gebautes Präfix `VP-` hat der Betreiber abgelehnt. Ab der 90 000. Variante werden die Nummern sechsstellig. Reine Zahlen können wie Lieferantennummern aussehen; die Oberfläche zeigt deshalb immer dazu, welche Nummer welche ist. Für Bestellungen beim Lieferanten muss die Oberfläche dessen Artikelnummer neben der eigenen zeigen.
