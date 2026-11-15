# velopim – PIM zwischen Lieferanten und Shopify

## Projektkontext
- Fahrrad-Onlineshop auf Shopify (Plan: Basic). Shopify bleibt Frontend, Checkout und Bestellsystem.
- velopim ist das führende System für Produktdaten: Lieferantendaten importieren, aufbereiten, freigeben, an Shopify synchronisieren.
- Erste Quelle: Hartje, vor allem Kompletträder. Weitere Lieferanten folgen, Quellen sind austauschbare Konnektoren.
- Im PIM liegt der gesamte Lieferantenkatalog, in den Store kommt nur eine Auswahl.
- Ein Nutzer (Shop-Betreiber, zugleich Entwickler). Projektsprache für Doku und Oberfläche: Deutsch. Code, Bezeichner und Commits: Englisch.

## Zusammenarbeit
- Auf Augenhöhe, Begründungen bei Architekturentscheidungen, keine Grundlagen-Erklärungen.
- Anforderungen hinterfragen, Risiken direkt ansprechen (Shopify-Limits, Datenqualität, Wartbarkeit).
- Wichtige Architekturentscheidungen nicht allein treffen: Optionen mit Vor- und Nachteilen vorlegen, Ergebnis unten eintragen.
- Subagenten nur für wirklich unabhängige Arbeit, vorher ankündigen. Sonnet für klar umrissene Umsetzung, Haiku für Suchen und Lesen.

## Leitplanken
- Niemals in den Live-Shop schreiben. Sync läuft standardmäßig im Dry-Run, echte Schreibzugriffe nur gegen den Development-Store (`SHOPIFY_WRITE_ENABLED=true` muss explizit gesetzt sein).
- Zugangsdaten nur über Umgebungsvariablen (`.env`, nicht eingecheckt; `.env.example` pflegen).
- Einfach und erweiterbar vor früh abstrahiert. Keine Abstraktion ohne zweiten Anwendungsfall, Ausnahme: Konnektor- und Task-Adapter-Schnittstelle.
- Interne Daten dürfen nie zu Shopify gelangen (siehe „Shopify-Projektion“).
- Oberfläche: klar, auf den Alltag des Shop-Betreibers reduziert.

## Tech-Stack
- Backend: Go, ein Binary. HTTP mit `net/http`, JSON-API unter `/api`.
- Datenbank: PostgreSQL. Zugriff mit `pgx` + `sqlc` (SQL in `db/queries/`, generierter Code in `internal/db/`, nicht von Hand ändern), Migrationen mit `goose`.
- Hintergrundjobs: River (Queue in PostgreSQL, kein Redis).
- Frontend: React + TypeScript + Vite in `web/`. Build-Ergebnis wird per `go:embed` ins Binary gepackt, in Produktion läuft kein Node.
- Shopify: Admin GraphQL API.
- Tests: Go-Standard (`go test`), tabellengetrieben. Pflicht für Regel-Engine, Mappings/Normalisierung, Preiskalkulation und Sync-Diff.

## Befehle
- Go und Node liegen unter `~/.local/bin` (ggf. `export PATH="$HOME/.local/bin:$PATH"`).
- Datenbank starten: `docker compose up -d` (liest `POSTGRES_PASSWORD` aus `.env`).
- Migrationen: `go run github.com/pressly/goose/v3/cmd/goose@v3.24.1 -dir db/migrations postgres "$DATABASE_URL" up`
- Tests ohne Datenbank: `go test ./...` (Datenbanktests werden übersprungen).
- Tests mit Datenbank: `TEST_DATABASE_URL="$DATABASE_URL" go test ./...` (jeder Test legt ein eigenes Schema an und löscht es wieder).
- Nach Änderungen an `db/queries/` oder `db/migrations/`: `sqlc generate` (sqlc liegt in `~/.local/bin`, ohne C-Compiler gebaut mit `CGO_ENABLED=0 go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`).
- Oberfläche bauen: in `web/` `npm install && npm run build`, danach `go build -o bin/velopim ./cmd/velopim`. Entwicklung: `npm run dev` (leitet `/api` an den Go-Server weiter).
- Server: `./bin/velopim serve` (braucht `DATABASE_URL`, z. B. `set -a; . ./.env; set +a`). Import: `./bin/velopim import <quelle> <datei>`.
- Beispielquelle für lokale Tests: `testdata/example_source.sql` in die Datenbank laden, dann `./bin/velopim import example testdata/example_supplier.csv`.
- Vor jedem Commit: `gofmt -l .` und `go vet ./...` müssen sauber sein.

## Stand
Fertig und getestet: Regel-Engine (`rules`), Preiskalkulation (`pricing`), Verfügbarkeit (`availability`), CSV-Konnektor (`connector/csvfile`), Änderungserkennung (`actions`), Shopify-Projektion und Diff (`shopify`), Datenbankschema, Import-Pipeline (`importer`), Task-Entscheidung (`tasks`), Variantengruppierung als reine Funktion (`catalog.Group`, noch nicht an Datenbank und Pipeline angebunden), HTTP-API für Tasks und Importläufe (`api`), Import-Befehl im Binary, React-Oberfläche mit Taskliste (Freigeben/Ablehnen), erledigten Aufgaben und Importläufen.
Noch nicht begonnen: eigener Katalog in der Datenbank (Produkt/Variante aus der Gruppierung anlegen, Trennmerkmale speichern), Übernahme freigegebener Aktionen in den Katalog, Nutzungsrichtlinie in der Pipeline, Shopify-API-Client, Produktansicht, Pflege von Regeln und Quellen in der Oberfläche, River-Jobs.

## Geplante Struktur
```
cmd/velopim/        Einstiegspunkt (server, worker, import-Befehle)
internal/connector/ Konnektor-Schnittstelle, hartje/
internal/catalog/   Produkt, Variante, Lieferantenartikel, Feldherkunft
internal/rules/     Regel-Engine
internal/actions/   Aktionen und ihre Ausführung
internal/tasks/     Tasks, Adapter-Schnittstelle
internal/pricing/   Kalkulation, Aktionen mit Zeitraum
internal/availability/
internal/shopify/   Projektion, Diff, Sync-Client
internal/api/       HTTP-Handler
db/migrations/, db/queries/
web/                React-App
testdata/           Beispiel-Lieferantendaten
```

## Datenmodell (Kern)
- `source`, `import_run`, `raw_record`: Originaldatei unverändert mit Hash abgelegt; pro Artikel neue Rohversion nur bei geändertem Inhalt.
- `supplier_item`: normalisierte Sicht je Quelle (Artikelnummer, EAN, EK, UVP, Bestand, Roh-Verfügbarkeit, Attribute).
- `product` (ein Produkt pro Modell und Modelljahr, Verweis auf Vorgänger/Nachfolger), `variant`; `variant_supplier_item` n:m mit bevorzugtem Lieferanten.
- `usage_policy`: je Quelle und Marke pro Feldgruppe (Bilder, Texte, Preise, Attribute) `use` / `internal_only` / `ignore`.
- `field_change`: Änderungsprotokoll pro Feldwert (Entität, Feld, alt, neu, Herkunft: Quelle+Importlauf / Regel / Person, Zeitpunkt, Begründung). Aktueller Stand in typisierten Spalten.
- `action`: Änderungsvorschlag mit Status `proposed → auto_applied | pending_approval → approved | rejected → synced`, inklusive greifender Regel.
- `rule`: Geltungsbereich (Lieferant, Marke, Kategorie), Aktionstyp, Bedingungen aus festem Katalog, Ergebnis (`auto` / `approval`). Spezifischste Regel gewinnt, im Zweifel Freigabe.
- `task`: Status, Priorität, Verweis auf Aktion oder Produkt, `external_ref` für spätere Adapter. Erledigen eines Tasks führt die Aktion aus oder verwirft sie.
- `price`: EK vom Lieferantenartikel, Kalkulationsregel je Marke/Kategorie (Aufschlag, Rundung), Aktionen mit Zeitraum überschreiben Verkaufs- und Streichpreis.
- `availability`: Status (`in_stock`, `orderable`, `preorder`, `on_request`, `unavailable`, `discontinued`), Lieferzeit in Tagen von/bis, Termin, Menge, Quelle, Stand. Konnektoren bilden per konfigurierbarem Mapping darauf ab.
- `internal_note`, interne Felder je Produkt und Lieferant: eigene Tabellen.
- `shopify_listing`: ob ein Produkt in den Store soll, Shopify-IDs, zuletzt synchronisierter Stand (Hash).

## Datenfluss
1. Konnektor holt Daten, legt sie roh ab.
2. Normalisierung zu `supplier_item`, Vergleich mit vorigem Stand.
3. Unterschiede werden Aktionen; Regel-Engine entscheidet automatisch oder Task.
4. Übernommene Aktionen ändern den Produktstand und schreiben `field_change`.
5. Shopify-Projektion berechnen, mit letztem Sync-Stand vergleichen.
6. Unterschiede als Sync-Jobs an Shopify (idempotent, Rate Limits, Retries). Fehler erzeugen einen Task.

## Architekturentscheidungen
| Nr. | Entscheidung | Begründung |
|---|---|---|
| 1 | Go statt Rust | Wartungsarm, ein Binary, schnelle Entwicklung für datenlastige Web-Anwendung |
| 2 | PostgreSQL | JSONB für Rohdaten, parallele Jobs, Queue in derselben Datenbank |
| 3 | React-Frontend (Wunsch des Betreibers) | Build-Kette mit Node wird in Kauf genommen; Laufzeit bleibt ein Go-Binary |
| 4 | Provenance: typisierte Spalten + `field_change`-Protokoll | Einfach abzufragen, vollständige Historie; kein Event Sourcing, kein generisches Attributmodell |
| 5 | Lieferantenartikel getrennt von Produkt/Variante | Mehrere Lieferanten pro Artikel ohne späteren Umbau |
| 6 | Shopify-Projektion als explizite Positivliste | Der Sync liest nur aus der Projektion, interne Felder kommen dort nicht vor |
| 7 | Feldhoheit statt „nie in Shopify bearbeiten“ | PIM besitzt definierte Felder, Shopify/Apps den Rest; Drift-Erkennung später |
| 8 | Verfügbarkeit: nur Statuswechsel sind Aktionen | Mengenänderungen werden pro Importlauf gesammelt protokolliert, sonst flutet es Aktionen und Historie |
| 9 | Bestand im MVP: nur „verkaufbar“ + Lieferzeit-Metafeld, keine Stückzahlen | Eigenbestand wird noch nicht geführt; später eigene Shopify-Location |
| 10 | Regeln strukturiert statt freier Regelsprache | Testbar, per Formular pflegbar |
| 11 | Matching gestuft: EAN → Herstellernummer + Marke → manuell per Task | EANs fehlen, sind falsch oder werden wiederverwendet |
| 12 | `supplier_item` zeigt immer den letzten Stand des Lieferanten; Aktionen steuern nur die Übernahme in den eigenen Katalog und zu Shopify | Lieferantensicht bleibt wahrheitsgetreu, Freigaben wirken dort, wo es öffentlich wird. Vom Betreiber bestätigt |
| 13 | Schutzregeln (`guard`): erzwingen Freigabe auch gegen spezifischere Regeln | Sonst hebelt eine Markenregel „unter Mindestmarge immer Freigabe“ aus. Bei Preisen derzeit ohne Wirkung, siehe Nr. 18 |
| 14 | Datenbankzugriff über `sqlc` (Wunsch des Betreibers) | Abfragen werden beim Generieren gegen das Schema geprüft; kein handgeschriebenes SQL im Go-Code |
| 15 | Import wird angehalten, wenn mehr als 20 % der aktiven Artikel fehlen | Abgeschnittene Lieferungen dürfen nichts deaktivieren; es entsteht ein Task |
| 16 | Eine neuere offene Aktion ersetzt die ältere für dasselbe Feld (`superseded`) | Tasks stapeln sich nicht bei wiederholten Änderungen |
| 17 | Datenprobleme eines Imports ergeben einen Sammel-Task, Details in `import_run.summary` | Tausende fehlende EANs dürfen die Taskliste nicht fluten |
| 18 | Preisänderungen gehen vorerst immer zur Freigabe, keine `auto`-Regeln für `price_change` anlegen. Senkt der Lieferant seinen Einkaufspreis um X €, enthält der Vorschlag den eigenen Verkaufspreis minus X € (betragsgleich, keine Neukalkulation per Aufschlag). Automatische Übernahme soll später je Regel zuschaltbar sein | Vorgabe des Betreibers. Folge: Kalkulation setzt den Verkaufspreis beim Anlegen, danach folgt er den Senkungen. Umsetzung erst mit dem eigenen Katalog, vorher gibt es keinen Verkaufspreis |
| 19 | API-Typen in `web/src/api.ts` von Hand gepflegt, passend zu `internal/api/api.go`. **Vorläufig, ohne Betreiber entschieden** | Vier Endpunkte rechtfertigen keine OpenAPI-Kette; bei wachsender API neu bewerten |
| 20 | Kein Login: Server lauscht nur auf localhost, schreibende Anfragen müssen JSON sein. **Vorläufig** | Ein Nutzer, lokaler Betrieb. Vor jedem Betrieb im Netz braucht es eine Anmeldung |
| 21 | Produktschnitt je Modell einstellbar: Standard ist ein Produkt pro Modell und Modelljahr mit Farbe und Rahmenhöhe als Optionen; je Modell können beliebige Merkmale (Farbe, Rahmenhöhe, Lieferantenattribute wie Rahmenform oder Antrieb) als Trennmerkmale Produkte trennen, die übrigen werden Optionen (Vorgabe des Betreibers) | Rahmenform oder Antrieb trennen im Fahrradhandel oft sinnvoller als Farbe. Umstellen nach dem ersten Sync legt Shopify-Produkte neu an (neue URLs) und soll deshalb einen Task mit Warnung erzeugen |

## MVP-Umfang
Hartje-Konnektor über Dateiimport mit Rohdatenablage · Normalisierung und Variantengruppierung · Nutzungsrichtlinie · Preiskalkulation · Regel-Engine (Neuprodukt, Preis, Statuswechsel Verfügbarkeit) · Taskliste mit Freigabe/Ablehnung · Produktansicht mit Feldherkunft und internen Notizen · Auswahl „in den Store“ · Shopify-Sync (Produkt, Varianten, Preise, Bilder, Lieferzeit-Metafeld) im Dry-Run und gegen den Development-Store.

Später: Matching der Bestandsprodukte · Aktionen mit Zeitraum · Eigenbestand · Drift-Erkennung · zweiter Lieferant · Task-Adapter (Todoist usw.) · Benachrichtigungen.

## Offene Punkte
- Hartje-Zugang und Datenformat unbekannt (CSV/BMEcat-Feed, veloconnect, API?). Keine Annahmen treffen; bis zur Klärung Dateischnittstelle mit Testdaten.
- Wie Hartje Varianten liefert (flach oder Eltern-Kind) und ob Modellname, Farbe und Rahmenform sauber genug für die Gruppierung sind. Die Gruppierung vergleicht Werte bisher exakt (nur Leerzeichen am Rand entfernt).
- Development-Store und API-Token fehlen noch.
- Wie das Theme die Lieferzeit anzeigt (Metafeld-Namespace und -Key).
- Erstimport eines ganzen Katalogs erzeugt pro Artikel einen Task „Neues Produkt prüfen“. Die Taskliste zeigt höchstens 200 Einträge; es fehlen Sammelfreigabe, Filter oder eine Regel für Neuprodukte.
- Jeder Import mit Datenproblemen legt einen neuen Sammel-Task an, auch wenn es dieselben Probleme wie beim letzten Lauf sind.
- Meldungen zu Datenproblemen und Regelnamen sind englisch, die Oberfläche übersetzt nur die Entscheidungsgründe.
- Freigegebene Aktionen ändern noch nichts am eigenen Katalog (der fehlt noch); die Freigabe wird nur protokolliert.
- Preiskalkulation: vorerst Aufschlag auf EK, später weitere Verfahren (Zielmarge ist schon da, „UVP übernehmen“ fehlt). Faktor, Rundung und UVP als Obergrenze sind noch nicht festgelegt.
- Zu Nr. 18: Was passiert bei EK-Erhöhungen des Lieferanten? Wird X € netto (wie der EK) oder brutto vom Verkaufspreis abgezogen?
- Zeichensatz der Lieferantendateien: der CSV-Konnektor erwartet UTF-8; Windows-1252 ist noch nicht unterstützt.
- Texte für die Lieferzeit-Anzeige stehen fest im Code (`availability.Derive`), sollen später konfigurierbar sein.
- `testdata/example_supplier.csv` ist erfunden und kein Hartje-Format.
