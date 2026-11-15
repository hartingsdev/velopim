# 2. PostgreSQL als Datenbank

**Status:** Angenommen (vom Betreiber bestätigt)

## Kontext
Rohdaten der Lieferanten haben je Quelle eine andere Form. Importe, Sync-Jobs und Oberfläche greifen parallel zu. Hintergrundjobs brauchen eine Queue.

## Entscheidung
PostgreSQL 17. Rohdaten und Konfigurationen liegen als JSONB, der Katalog in typisierten Spalten. Die Job-Queue (River) liegt in derselben Datenbank.

## Alternativen
- **SQLite:** einfacher im Betrieb, aber schwächer bei parallelen Schreibzugriffen und ohne passende Job-Queue.
- **Eigene Queue mit Redis:** ein zweiter Dienst, der betrieben werden muss.

## Folgen
Ein Dienst zum Betreiben und Sichern. Lokal läuft die Datenbank über `docker compose`.
