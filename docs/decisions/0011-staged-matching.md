# 11. Matching gestuft: EAN, dann Herstellernummer, dann manuell

**Status:** Angenommen, noch nicht gebaut

## Kontext
Bestehende Shopify-Produkte und Artikel mehrerer Lieferanten müssen einander zugeordnet werden. EANs fehlen, sind falsch oder werden wiederverwendet.

## Entscheidung
Erst über die EAN, dann über Herstellernummer plus Marke, sonst ein Task zur manuellen Zuordnung.

## Alternativen
- **Nur EAN:** zu viele Lücken und Fehltreffer.
- **Unscharfer Namensvergleich:** falsche Zuordnungen sind teurer als ein Task.

## Folgen
Zweifelsfälle landen beim Betreiber. Das Matching der Bestandsprodukte steht nach dem MVP an.
