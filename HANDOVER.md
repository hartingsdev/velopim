# Übergabe velopim

Stand: 22.11.2026, Zweig `mvp-foundation`, letzter Code-Commit `1c2a286`. Arbeitsverzeichnis sauber, nichts gepusht, kein Pull Request.

Diese Datei ist eine Momentaufnahme. Gepflegt werden [CLAUDE.md](CLAUDE.md) (Regeln, Befehle) und [docs/](docs/README.md) (Architektur, Datenmodell, Stand, offene Punkte, Entscheidungen).

## Ziel
velopim ist das PIM zwischen Lieferanten und dem Shopify-Shop (Fahrräder, Plan Basic). Shopify bleibt Shop und Checkout, velopim ist führend für Produktdaten: Lieferantendaten importieren, aufbereiten, freigeben, an Shopify synchronisieren. Erster Lieferant ist Hartje (vor allem Kompletträder), weitere folgen als Konnektoren. Ein Nutzer: Shop-Betreiber und zugleich Entwickler.

## Erledigt
Der ganze Ablauf Import → Aufgaben → Katalog → Shopify läuft, bisher nur mit erfundenen Beispieldaten und gegen den Development-Store `fahrrad-ass-dev`.

- **Import:** CSV-Konnektor (Spalten, Trennzeichen, Zeichensatz einstellbar), Upload, Befehl oder zeitgesteuerter Abruf einer Datei per Job. Rohdaten werden abgelegt. Halt bei auffällig vielen fehlenden Artikeln.
- **Aufgaben:** Änderungen werden Aktionen; Regeln entscheiden automatisch oder verlangen Freigabe. Kategorien, Suche, Filter, Wiedervorlage, tägliche Mail.
- **Katalog:** Ein neues Modell ist eine Aufgabe. Freigabe legt Produkte und Varianten mit eigener Artikelnummer (ab 10001) und kalkuliertem Preis an. Trennung in Shop-Produkte je Modell wählbar. EK-Änderungen schlagen denselben Betrag beim Verkaufspreis vor.
- **Oberfläche:** Aufgaben, Produkte (je Modell), Importe, Shopify-Vorschau, Lieferanten, Regeln, Kalkulation, übergreifende Suche.
- **Shopify-Sync:** `productSet` je Produkt, neue Produkte als Entwurf mit Aufgabe „Veröffentlichen“, aktive Produkte werden im Kanal „Onlineshop“ veröffentlicht, Bilder, Lieferzeit-Metafeld, Fehler als Aufgabe. Automatisch per Job nach Import, Freigabe und Änderung.
- **Textvorschläge:** Knopf „Beschreibung vorschlagen“ in der Produktansicht (Claude API). Gespeichert wird erst nach Prüfung durch den Betreiber.
- **Zuletzt:** Optionswerte, die sich nur in Groß- und Kleinschreibung unterscheiden, gelten als ein Wert.

Die vollständige Liste steht in [docs/status.md](docs/status.md).

### Was geprüft ist und was nicht
- Alle Go-Tests laufen grün, auch die Datenbanktests.
- Der Sync ist gegen den Development-Store geprüft (anlegen, wiederholen, Varianten, Verfügbarkeit, Bilder, Veröffentlichen im Kanal).
- **Nicht geprüft:** Textvorschläge gegen die echte Claude API (kein `ANTHROPIC_API_KEY` in der `.env`, nur gegen einen lokalen Platzhalter-Server). Mailversand gegen einen echten Mailserver. Alles mit echten Hartje-Daten.

## Offen
### Hartje anbinden (größter Block)
Hartje verweist auf zwei Wege. Beides ist recherchiert, nichts davon gebaut.

- **DST** für den Katalog (Stammdaten, Bilder). Datenplattform der Branche, getragen von der Stichting DST. REST mit JSON unter `https://dst3-api.platformdst.nl/api/customer/…`, Anmeldung per Benutzername und Passwort gegen ein Token (Keycloak). `/api/customer/product` filtert nach Lieferant, Marke, EAN, Artikelnummer und „geändert seit“. Es fehlen: ein Konto, die Feldliste („DST JSON schema 3.1“, API-Handbuch für Abnehmer, beides in der Kennisbank), Angaben zu Blättern und Limits, die Nutzungsrechte für Texte und Bilder.
- **veloconnect** für Einkaufspreise und Verfügbarkeit. Hartjes Server: `https://api.hartje.veloconnect.io/`. Abfragen je bekannter Artikelnummer (`GetItemDetailsList`), kein Katalogabzug. Es fehlen: Zugangsdaten für die Schnittstelle, Version und unterstützte Vorgänge (zeigt `GetProfile`).
- Beide brauchen einen eigenen Konnektor: Der heutige Abruf lädt nur eine einzelne Datei. Wie ein API-Lieferant eingehängt wird, ist eine Architekturentscheidung, die noch aussteht.

Quellen: [User / POS API 3.0](https://www.platformdst.com/en/knowledge-base/user-pos-api-3-0/), [Konto beantragen](https://www.platformdst.com/en/request-an-account/), [DST bei Hartje](https://hartje.de/services/dst-plattform/), [veloconnect-Dokumentation](https://veloconnect.atlassian.net/wiki/spaces/DOKUMENTAT/overview), [Liste der veloconnect-Server](https://www.veloconnect.de/en/implementation).

### Vom Betreiber zu entscheiden
- **Textvorschläge:** Sollen auch Name, SEO-Felder oder Bildtexte erzeugt werden? Tonfall und Anrede? Automatischer Vorschlag mit Aufgabe „Text prüfen“ beim Anlegen? Welches Modell?
- **Fehlender Optionswert:** Soll er das Anlegen eines Modells sperren, oder reicht der Hinweis?
- **Livegang:** Der Client schreibt nur in Development-Stores. Wann und wie wird der Live-Shop freigeschaltet, und wie werden die dort vorhandenen Produkte zugeordnet?
- **Lieferzeit im Theme:** Namespace und Key des Metafelds (vorläufig `velopim.delivery_time`).
- **Kalkulation:** Echte Werte fehlen; in der lokalen Datenbank steht nur eine Testregel.
- **OIDC mit Authentik** und Zuweisung von Aufgaben an Gruppen: Serveradresse, Client, Gruppennamen, Filter oder Rechte.

### Noch nicht gebaut
Bilder und Varianten von Hand bearbeiten · Matching bestehender Shopify-Produkte · andere Formate als CSV · Ansicht für Jobs · Sammelfreigabe für viele Aufgaben · Beschreibungen für viele Produkte auf einmal.

Alle offenen Punkte und bekannten Schwächen stehen in [docs/open-questions.md](docs/open-questions.md).

## Wichtige Entscheidungen
Jede Entscheidung hat eine eigene Datei in [docs/decisions/](docs/decisions/README.md) (1 bis 32). Die wichtigsten:

| Nr. | Entscheidung |
|---|---|
| 6 | Zu Shopify geht nur eine Positivliste (Projektion); interne Daten nie. |
| 9, 27 | Keine Stückzahlen, kein Eigenbestand. Kaufbar ist, was der Lieferant liefern kann; Verfügbarkeit wird live gelesen. |
| 18, 26 | Preise ändern sich nur per Freigabe oder Regel; eine EK-Änderung wird betragsgleich vorgeschlagen. |
| 21, 22 | Ein Task je neuem Modell; die Trennung in Shop-Produkte ist je Modell wählbar. |
| 23 | Eigene, kurze Artikelnummern ohne Präfix. |
| 25 | Neue Produkte gehen als Entwurf zu Shopify; Veröffentlichen ist eine Aufgabe. |
| 28 | Nutzungsrichtlinie je Lieferant, Marke und Datengruppe (verwenden, nur intern, ignorieren). |
| 29 | Sync über `productSet`, nur in Development-Stores, Veröffentlichung im Kanal „Onlineshop“. Vorläufig. |
| 30 | Abholen und Sync laufen als Jobs (River) im Serverprozess. |
| 32 | Produktbeschreibungen nur als Vorschlag; an das Textmodell geht eine Positivliste. Vorläufig. |

Als „vorläufig“ markierte Entscheidungen sind ohne den Betreiber getroffen und noch zu bestätigen.

### Leitplanken
- Niemals in den Live-Shop schreiben. In der `.env` bleibt `SHOPIFY_WRITE_ENABLED=false`; die Sperre im Client für Nicht-Development-Stores bleibt.
- Zugangsdaten nur in der `.env`, nie im Chat oder im Repository. Abruf-Einstellungen nennen nur Variablen mit dem Präfix `VELOPIM_FETCH_`.
- Keine Annahmen über Hartje-Format und -Zugang.
- Kein handgeschriebenes SQL im Go-Code; `internal/db/` ist generiert.

## Nächste Schritte
1. **DST-Konto beantragen** (mit API-Nutzung, möglichst auch Testumgebung) und JSON-Schema 3.1 sowie API-Handbuch für Abnehmer in den Projektordner legen.
2. **veloconnect-Zugang bei Hartje anfragen.** Zugangsdaten in die `.env`, dann als Erstes `GetProfile` abfragen (liest nur).
3. **Entscheiden, wie API-Lieferanten eingehängt werden** (Optionen mit Vor- und Nachteilen vorlegen), dann DST-Konnektor bauen, danach veloconnect für Preise und Verfügbarkeit.
4. **`ANTHROPIC_API_KEY` eintragen** und die Textvorschläge mit echten Produkten prüfen.
5. **Offene Entscheidungen oben klären**, vor allem Livegang und Matching.

## Starten und testen
Go, Node und sqlc liegen unter `~/.local/bin`. `velopim` liest die `.env` im aktuellen Ordner selbst.

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Datenbank starten:

```bash
docker compose up -d
```

Oberfläche und Binary bauen:

```bash
(cd web && npm install && npm run build) && go build -o bin/velopim ./cmd/velopim
```

Server starten, danach http://localhost:8080 öffnen:

```bash
./bin/velopim serve
```

Verbindung zu Shopify prüfen (liest nur, zeigt Store, Rechte und Schreibschalter):

```bash
./bin/velopim shopify-check
```

Tests ohne Datenbank:

```bash
go test ./...
```

Tests mit Datenbank:

```bash
set -a; . ./.env; set +a; TEST_DATABASE_URL="$DATABASE_URL" go test ./...
```

Schreibender Test gegen den Development-Store (Schalter nur am Befehl setzen):

```bash
set -a; . ./.env; set +a; SHOPIFY_LIVE_TEST=1 SHOPIFY_WRITE_ENABLED=true go test ./internal/shopify -run Live -v
```

Vor jedem Commit:

```bash
gofmt -l . && go vet ./...
```

Weitere Befehle (Migrationen, `sqlc generate`, Beispieldaten laden, Import per Befehl, `velopim sync`, `velopim digest`) stehen in [CLAUDE.md](CLAUDE.md).

### Zustand der lokalen Umgebung
- Die lokale Datenbank enthält nur Testdaten: Beispiel-Lieferant, Modelle „City 3 Wave“ und „Gravel 2“, eine Testregel für die Kalkulation, einen angehaltenen Importlauf.
- Im Development-Store liegt „City 3 Wave rot“ als Entwurf; in velopim wartet dazu die Aufgabe „Veröffentlichen“.
- In der `.env` fehlen `ANTHROPIC_API_KEY` und die Mail-Einstellungen (`VELOPIM_SMTP_…`, `VELOPIM_MAIL_…`).
