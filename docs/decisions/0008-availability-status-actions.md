# 8. Verfügbarkeit: nur Statuswechsel sind Aktionen

**Status:** Angenommen

## Kontext
Bestandsmengen ändern sich bei jedem Import an vielen Artikeln.

## Entscheidung
Nur ein Wechsel des Status (z. B. auf Lager → nicht verfügbar) erzeugt eine Aktion. Mengenänderungen werden je Importlauf gezählt und in dessen Zusammenfassung abgelegt.

## Alternativen
- **Jede Mengenänderung als Aktion:** flutet Aktionen, Historie und Taskliste.

## Folgen
Die Mengenhistorie je Artikel ist nicht einzeln nachvollziehbar, nur der letzte Stand und die Anzahl der Änderungen je Lauf.
