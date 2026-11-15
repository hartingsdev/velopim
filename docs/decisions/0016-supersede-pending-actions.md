# 16. Eine neuere offene Aktion ersetzt die ältere

**Status:** Angenommen

## Kontext
Ändert sich dasselbe Feld mehrmals, bevor der Betreiber entschieden hat, stapeln sich sonst Aktionen und Tasks.

## Entscheidung
Kommt eine neue Aktion für denselben Artikel, Typ und dasselbe Feld, wird die noch offene ältere auf `superseded` gesetzt und ihr Task geschlossen.

## Alternativen
- **Alle Aktionen offen lassen:** der Betreiber müsste veraltete Vorschläge einzeln ablehnen.

## Folgen
Die ersetzte Aktion bleibt in der Historie. Der Alt-Wert der neuen Aktion ist der letzte Lieferantenstand, nicht der zuletzt freigegebene; das muss die Übernahme in den Katalog berücksichtigen.
