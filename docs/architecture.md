# Architektur

velopim steht zwischen den Lieferanten und Shopify. Shopify bleibt Shop, Checkout und Bestellsystem. velopim ist das führende System für Produktdaten: Lieferantendaten importieren, aufbereiten, freigeben und an Shopify synchronisieren. Im PIM liegt der ganze Lieferantenkatalog, in den Store kommt nur eine Auswahl.

## Tech-Stack
| Bereich | Wahl | Entscheidung |
|---|---|---|
| Backend | Go, ein Binary, HTTP mit `net/http`, JSON-API unter `/api` | [1](decisions/0001-go-backend.md) |
| Datenbank | PostgreSQL 17, Migrationen mit `goose` | [2](decisions/0002-postgresql.md) |
| Datenbankzugriff | `pgx` + `sqlc`: SQL in `db/queries/`, generierter Code in `internal/db/` | [14](decisions/0014-sqlc.md) |
| Hintergrundjobs | River, Queue in PostgreSQL (geplant, noch nicht eingebaut) | [2](decisions/0002-postgresql.md) |
| Oberfläche | React + TypeScript + Vite in `web/`, per `go:embed` im Binary | [3](decisions/0003-react-frontend.md) |
| Shopify | Admin GraphQL API (Client noch nicht gebaut) | |
| Tests | `go test`, tabellengetrieben | |

Tests sind Pflicht für Regel-Engine, Mappings und Normalisierung, Preiskalkulation, Gruppierung und Sync-Diff. Datenbanktests laufen nur mit `TEST_DATABASE_URL` und legen je Test ein eigenes Schema an (`internal/dbtest`).

## Aufbau
```
cmd/velopim/             Einstiegspunkt: serve, import
internal/connector/      Konnektor-Schnittstelle, Rohdatensatz, Lieferantenartikel
internal/connector/csvfile/  konfigurierbarer CSV-Konnektor
internal/availability/   Verfügbarkeitsmodell, Mapping aus Rohdaten, Anzeige
internal/pricing/        Preiskalkulation
internal/rules/          Regel-Engine
internal/actions/        Änderungserkennung zwischen zwei Importen
internal/importer/       Import-Pipeline (Datenbank)
internal/tasks/          Entscheidung über Tasks
internal/catalog/        Gruppierung von Lieferantenartikeln zu Produkten
internal/shopify/        Projektion und Diff (Sync-Client fehlt noch)
internal/api/            HTTP-Handler
internal/db/             von sqlc generiert, nicht von Hand ändern
internal/dbtest/         Hilfspaket für Datenbanktests
db/migrations/           goose-Migrationen
db/queries/              SQL für sqlc
web/                     React-App, embed.go liefert sie aus
testdata/                erfundene Beispieldaten
docs/                    diese Dokumentation
```

Die Fachlogik (`rules`, `pricing`, `availability`, `actions`, `catalog`, `shopify`) ist rein: keine Datenbank, keine Uhr, keine Netzwerkzugriffe. Nur `importer`, `tasks` und `api` sprechen mit der Datenbank.

## Datenfluss
1. **Holen:** Ein Konnektor liest die Lieferung. Die Originaldatei wird unverändert mit Hash abgelegt.
2. **Normalisieren:** Jeder Datensatz wird ein `supplier_item`. Eine neue Rohversion entsteht nur, wenn sich der Inhalt geändert hat. Die Verfügbarkeit wird über das Mapping der Quelle bestimmt.
3. **Vergleichen:** Unterschiede zum vorigen Stand werden Aktionen. Die Regel-Engine entscheidet: automatisch oder Freigabe. Eine Freigabe wird ein Task.
4. **Übernehmen:** Freigegebene und automatische Aktionen ändern den eigenen Katalog und schreiben `field_change`. *Noch nicht gebaut.*
5. **Projizieren:** Aus dem Katalog wird die Shopify-Projektion berechnet und mit dem letzten Sync-Stand verglichen.
6. **Synchronisieren:** Unterschiede gehen als Jobs an Shopify (idempotent, Rate Limits, Wiederholungen). Fehler erzeugen einen Task. *Noch nicht gebaut.*

Schutzmechanismen im Import: Halt bei auffällig vielen fehlenden Artikeln ([15](decisions/0015-hold-import-on-drop.md)), Ersetzen veralteter offener Aktionen ([16](decisions/0016-supersede-pending-actions.md)), Sammel-Task für Datenprobleme ([17](decisions/0017-collective-data-issue-task.md)).

## Leitplanken
- **Kein Schreiben in den Live-Shop.** Der Sync läuft standardmäßig im Dry-Run. Echte Schreibzugriffe gehen nur gegen den Development-Store und nur mit `SHOPIFY_WRITE_ENABLED=true`.
- **Zugangsdaten nur über Umgebungsvariablen** (`.env`, nicht eingecheckt; `.env.example` pflegen).
- **Interne Daten gelangen nie zu Shopify:** Der Sync liest nur aus der Projektion ([6](decisions/0006-shopify-projection-allowlist.md)).
- **Kein Login:** Der Server lauscht nur auf localhost ([20](decisions/0020-no-login-localhost.md)).

## API
| Methode und Pfad | Zweck |
|---|---|
| `GET /api/health` | Lebenszeichen |
| `GET /api/tasks?status=open\|closed` | Taskliste (höchstens 200) mit Aktion, Artikel und Zählern |
| `POST /api/tasks/{id}/decision` | `{"approve": bool, "note": string}`; 409, wenn der Task nicht mehr offen ist |
| `GET /api/import-runs` | die letzten 50 Importläufe mit Zusammenfassung |

Die TypeScript-Typen dazu stehen in `web/src/api.ts` und werden von Hand gepflegt ([19](decisions/0019-handwritten-api-types.md)).
