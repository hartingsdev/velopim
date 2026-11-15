# 28. Nutzungsrichtlinie je Lieferant, Marke und Datengruppe

**Status:** Angenommen (Wunsch des Betreibers: „einstellen können, was ich aus den Daten verwenden darf“), umgesetzt; die genaue Bedeutung der drei Stufen ist vorläufig

## Kontext
Nicht alles, was ein Lieferant liefert, darf oder soll im eigenen Shop erscheinen. Manche Marken erlauben die Nutzung ihrer Fotos oder Texte nicht.

## Entscheidung
Je Lieferant gibt es für vier Datengruppen (Bilder, Texte, Preise, Merkmale) eine Richtlinie, für alle Marken und mit Ausnahmen je Marke. Die Ausnahme für eine Marke gewinnt. Ohne Eintrag gilt „verwenden“.

| Stufe | Lieferantensicht | Aufgaben bei Änderungen | Übernahme ins Produkt |
|---|---|---|---|
| verwenden (`use`) | aktuell | ja, nach Regeln | ja |
| nur intern (`internal_only`) | aktuell | ja, nach Regeln | nein |
| ignorieren (`ignore`) | aktuell | nein | nein |

Die Richtlinie greift an drei Stellen: beim Anlegen eines Produkts aus einem Modell (Beschreibung, Bilder), bei der Übernahme freigegebener Änderungen und im Import (keine Aktion bei „ignorieren“). Gepflegt wird sie im Reiter „Lieferanten“.

## Alternativen
- **Nur an/aus:** einfacher, aber „sehen, aber nicht verwenden“ war der eigentliche Wunsch.
- **Filter erst in der Shopify-Projektion:** Die Daten stünden im Produkt und könnten über einen anderen Weg hinausgelangen. So kommen sie gar nicht erst hinein.

## Folgen
- Die Richtlinie wirkt auf künftige Übernahmen. Daten, die schon im Produkt stehen, entfernt sie nicht.
- „Nur intern“ erzeugt weiter Aufgaben, deren Freigabe nichts ändert. Wer das nicht will, wählt „ignorieren“.
- „Ignorieren“ bei Preisen schaltet auch die betragsgleiche Preisänderung ab ([18](0018-price-changes-need-approval.md)); bei Merkmalen entfallen auch die Hinweise zur Verfügbarkeit.
- Neue Modelle und entfallene Artikel erzeugen immer Aufgaben, unabhängig von der Richtlinie.
- Ob der Shop Bilder und Texte von Hartje verwenden darf, muss der Betreiber klären.
