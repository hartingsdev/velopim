# 26. Regeln entscheiden, ob Preise dem Einkaufspreis folgen

**Status:** Angenommen (Vorgabe des Betreibers); in der Regel-Engine umgesetzt, Pflege der Regeln in der Oberfläche fehlt

## Kontext
Ergänzt [18](0018-price-changes-need-approval.md). Der Betreiber will Regeln definieren, die EK-Änderungen automatisch an den Verkaufspreis weitergeben. Bei manchen Rädern soll eine Erhöhung mitgegangen werden, bei anderen nicht, etwa weil noch Lagerbestand zum alten EK da ist.

## Entscheidung
Ob eine EK-Änderung betragsgleich übernommen wird, entscheidet die Regel-Engine je Lieferant, Marke und Kategorie, getrennt für Senkung und Erhöhung. Ohne passende Regel bleibt es bei der Freigabe per Task mit fertigem Preisvorschlag.

## Alternativen
- **Immer Freigabe:** sicher, aber viel Handarbeit bei Preisrunden.
- **Immer automatisch:** nimmt die Kontrolle bei Erhöhungen.

## Folgen
Die Bedingung „noch Lagerbestand da“ lässt sich nicht als Regel abbilden, solange der Eigenbestand nicht erfasst wird ([9](0009-no-stock-quantities-mvp.md)). Bis dahin hilft nur eine Ausnahme je Produkt oder die Freigabe von Hand. Die Regel-Engine kennt dafür den Fakt `cost_change_cents` (negativ bei Senkung). Eine Regel „Senkungen automatisch“ ist `price_change` mit der Bedingung `cost_change_cents < 0` und dem Ergebnis `auto`. Regeln lassen sich bisher nur direkt in der Datenbank anlegen.
