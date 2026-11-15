# 4. Feldherkunft: typisierte Spalten plus Änderungsprotokoll

**Status:** Angenommen (Betreiber war unsicher, hat dem Vorschlag gefolgt)

## Kontext
Für jeden Feldwert soll nachvollziehbar sein, woher er kommt und wie er sich geändert hat.

## Entscheidung
Der aktuelle Stand liegt in normalen, typisierten Spalten. Jede Änderung eines Feldwerts schreibt zusätzlich eine Zeile in `field_change` (Entität, Feld, alt, neu, Herkunft: Import mit Lauf / Regel / Person / System, Zeitpunkt, Begründung). Das Protokoll wird nur ergänzt, nie geändert.

## Alternativen
- **Event Sourcing:** vollständige Historie, aber jeder Lesezugriff braucht Projektionen; zu viel Aufwand für eine Person.
- **Generisches Attributmodell (EAV):** flexibel, aber Abfragen und Typprüfung werden mühsam.

## Folgen
Der Stand ist einfach abzufragen, die Historie vollständig. Preis: Wer ein Feld ändert, muss das Protokoll mitschreiben; das passiert in der Pipeline an einer Stelle.
