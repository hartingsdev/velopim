# 12. Lieferantensicht zeigt immer den letzten Stand

**Status:** Angenommen (vom Betreiber bestätigt)

## Kontext
Offen war, ob eine noch nicht freigegebene Änderung schon in `supplier_item` stehen darf.

## Entscheidung
`supplier_item` wird bei jedem Import sofort aktualisiert. Aktionen und Freigaben steuern nur, was in den eigenen Katalog und zu Shopify übernommen wird.

## Alternativen
- **Lieferantensicht erst nach Freigabe ändern:** die Sicht würde nicht mehr zeigen, was der Lieferant tatsächlich liefert, und der Vergleich beim nächsten Import würde unklar.

## Folgen
Zwischen Lieferantensicht und eigenem Katalog kann bewusst ein Unterschied bestehen, solange eine Freigabe aussteht.
