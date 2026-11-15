# 24. Kalkulation in der Oberfläche pflegen, Preise im Freigabe-Task zeigen

**Status:** Angenommen (Vorgabe des Betreibers)

## Kontext
Faktor, Rundung und UVP-Obergrenze waren nicht festgelegt. Der Betreiber will sie selbst einstellen und bei der Entscheidung die UVP sehen.

## Entscheidung
Kalkulationsregeln (`pricing_rule`) werden in der Oberfläche je Marke und Kategorie gepflegt: Verfahren, Wert, Mehrwertsteuer, Rundung, UVP als Obergrenze. Die spezifischste Regel gilt. Der Freigabe-Task eines Modells zeigt je Variante EK, UVP und den kalkulierten Verkaufspreis. Gibt es keine passende Regel, wird die Variante ohne Verkaufspreis angelegt.

## Alternativen
- **UVP als Startpreis ohne Regel:** bequem, aber ein stiller Standard, den der Betreiber nicht gewählt hat.
- **Feste Werte im Code:** nicht pflegbar.

## Folgen
Eine Variante ohne Preis kann nicht in den Store. Der Preis wird beim Anlegen einmal kalkuliert und danach nicht mehr automatisch neu berechnet ([18](0018-price-changes-need-approval.md)).
