# 10. Regeln strukturiert statt freier Regelsprache

**Status:** Angenommen

## Kontext
Der Betreiber soll selbst festlegen, was automatisch übernommen wird und was eine Freigabe braucht, je Lieferant, Marke und Kategorie.

## Entscheidung
Eine Regel hat einen Geltungsbereich, einen Aktionstyp, Bedingungen aus einem festen Katalog von Fakten (z. B. Preisänderung in Prozent, Marge) und ein Ergebnis (`auto` oder `approval`). Die spezifischste Regel gewinnt. Bei Gleichstand mit Widerspruch und ohne passende Regel gilt Freigabe. Die greifende Regel wird an der Aktion gespeichert.

## Alternativen
- **Freie Ausdruckssprache:** mächtiger, aber schwer zu testen, schwer per Formular zu pflegen und eine Fehlerquelle.

## Folgen
Neue Arten von Bedingungen brauchen eine Codeänderung (neuer Fakt). Dafür ist jede Regel testbar und in einem Formular darstellbar.
