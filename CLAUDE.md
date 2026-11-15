# velopim – PIM zwischen Lieferanten und Shopify

Fahrrad-Onlineshop auf Shopify (Plan: Basic). velopim ist das führende System für Produktdaten: Lieferantendaten importieren, aufbereiten, freigeben, an Shopify synchronisieren. Erste Quelle ist Hartje (vor allem Kompletträder), weitere Lieferanten folgen als austauschbare Konnektoren. Ein Nutzer: Shop-Betreiber und zugleich Entwickler.

## Dokumentation
Diese Datei bleibt kurz. Alles Ausführliche steht in `docs/`:
- [docs/architecture.md](docs/architecture.md): Tech-Stack, Aufbau, Datenfluss, API
- [docs/data-model.md](docs/data-model.md): Tabellen und ihre Bedeutung
- [docs/status.md](docs/status.md): fertig, offen, MVP-Umfang
- [docs/open-questions.md](docs/open-questions.md): ungeklärte Fragen, bekannte Schwächen
- [docs/decisions/](docs/decisions/README.md): Architekturentscheidungen, je eine Datei

Pflege:
- Neue Architekturentscheidung: eigene Datei `docs/decisions/NNNN-titel.md` (Status, Kontext, Entscheidung, Alternativen, Folgen) und Zeile im Index. Nicht hier eintragen.
- Nach jeder Arbeit `docs/status.md` und `docs/open-questions.md` aktualisieren.
- Vor Arbeit an einem Bereich die passenden Dateien in `docs/` lesen.

## Zusammenarbeit
- Auf Augenhöhe, Begründungen bei Architekturentscheidungen, keine Grundlagen-Erklärungen.
- Anforderungen hinterfragen, Risiken direkt ansprechen (Shopify-Limits, Datenqualität, Wartbarkeit).
- Wichtige Architekturentscheidungen nicht allein treffen: Optionen mit Vor- und Nachteilen vorlegen. Was ohne Betreiber entschieden wurde, als „vorläufig“ kennzeichnen.
- Subagenten nur für wirklich unabhängige Arbeit, vorher ankündigen. Sonnet für klar umrissene Umsetzung, Haiku für Suchen und Lesen.
- Sprache: Dokumentation und Oberfläche deutsch; Code, Bezeichner, Dateinamen und Commits englisch.

## Leitplanken
- Niemals in den Live-Shop schreiben. Sync läuft standardmäßig im Dry-Run, echte Schreibzugriffe nur gegen den Development-Store `fahrrad-ass-dev` (`SHOPIFY_WRITE_ENABLED=true` muss explizit gesetzt sein). Die Sperre im Client für Nicht-Development-Stores nicht entfernen oder umgehen.
- Zugangsdaten nur über Umgebungsvariablen (`.env`, nicht eingecheckt; `.env.example` pflegen).
- Interne Daten dürfen nie zu Shopify gelangen: Der Sync liest nur aus der Projektion in `internal/shopify`.
- Hartje-Format und -Zugang sind unbekannt: keine Annahmen treffen.
- Einfach und erweiterbar vor früh abstrahiert. Keine Abstraktion ohne zweiten Anwendungsfall, Ausnahme: Konnektor- und Task-Adapter-Schnittstelle.
- Kein handgeschriebenes SQL im Go-Code: Abfragen in `db/queries/`, `internal/db/` ist generiert und wird nicht von Hand geändert.
- Tests sind Pflicht für Regel-Engine, Mappings/Normalisierung, Preiskalkulation, Gruppierung und Sync-Diff (tabellengetrieben).
- Oberfläche: klar, auf den Alltag des Shop-Betreibers reduziert.

## Befehle
- Go, Node und sqlc liegen unter `~/.local/bin` (ggf. `export PATH="$HOME/.local/bin:$PATH"`).
- Umgebung laden: `set -a; . ./.env; set +a`
- Datenbank starten: `docker compose up -d`
- Migrationen: `go run github.com/pressly/goose/v3/cmd/goose@v3.24.1 -dir db/migrations postgres "$DATABASE_URL" up`
- Nach Änderungen an `db/queries/` oder `db/migrations/`: `sqlc generate`
- Tests ohne Datenbank: `go test ./...`; mit Datenbank: `TEST_DATABASE_URL="$DATABASE_URL" go test ./...`
- Oberfläche bauen: in `web/` `npm install && npm run build`, danach `go build -o bin/velopim ./cmd/velopim`. Entwicklung: `npm run dev`.
- Server: `./bin/velopim serve`. Import: `./bin/velopim import <quelle> <datei>`. Shopify: `./bin/velopim shopify-check` (liest nur), `./bin/velopim sync`.
- Schreibender Test gegen den Development-Store: `SHOPIFY_LIVE_TEST=1 SHOPIFY_WRITE_ENABLED=true go test ./internal/shopify -run Live -v`. Den Schalter nur am Befehl setzen, nicht in der `.env`.
- Typen der Admin-API nachsehen statt raten: `go run ./cmd/introspect ProductSetInput` (liest nur).
- Beispieldaten: `testdata/example_source.sql` laden, dann `./bin/velopim import example testdata/example_supplier.csv`.
- Vor jedem Commit: `gofmt -l .` und `go vet ./...` müssen sauber sein.
