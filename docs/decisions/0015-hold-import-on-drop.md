# 15. Import anhalten, wenn mehr als 20 % der Artikel fehlen

**Status:** Angenommen

## Kontext
Fehlt ein Artikel in einer Lieferung, wird er deaktiviert. Eine abgeschnittene oder fehlerhafte Datei würde so große Teile des Katalogs deaktivieren.

## Entscheidung
Fehlen mehr als 20 % der bisher aktiven Artikel, bekommt der Lauf den Status `held`: Vorhandene Artikel werden aktualisiert, es wird nichts deaktiviert, und es entsteht ein Task mit hoher Priorität.

## Alternativen
- **Immer deaktivieren:** ein kaputter Feed nimmt Produkte aus dem Shop.
- **Nie automatisch deaktivieren:** ausgelaufene Artikel bleiben verkaufbar.

## Folgen
Die Schwelle ist ein Parameter (`MaxDropPct`). Bei einem echten Sortimentswechsel bestätigt der Betreiber den Lauf im Task („Fehlende Artikel deaktivieren“): Die abgelegte Originaldatei wird ohne die Prüfung erneut importiert. Das geht nur, solange der angehaltene Lauf der neueste der Quelle ist; ein neuerer Import schließt den Task.
