# Stand

## Fertig und getestet
| Baustein | Paket |
|---|---|
| Regel-Engine | `rules` |
| Preiskalkulation (Aufschlag, Zielmarge, Rundung, UVP-Deckel) | `pricing` |
| Verfügbarkeitsmodell mit Mapping | `availability` |
| CSV-Konnektor | `connector/csvfile` |
| Änderungserkennung | `actions` |
| Import-Pipeline mit Rohdatenablage, Aktionen und Tasks; neue Artikel als ein Task je Modell | `importer` |
| Task-Entscheidung | `tasks` |
| Gruppierung zu Produkten mit Trennmerkmalen | `catalog` |
| Eigener Katalog: Freigabe eines Modells legt Produkte (Entwurf) und Varianten mit eigener Artikelnummer und kalkuliertem Preis an | `catalog`, `tasks` |
| Shopify-Projektion und Diff | `shopify` |
| Betragsgleiche Preisänderung: EK-Änderung schlägt denselben Betrag beim Verkaufspreis vor; Freigabe oder Regel ändert den Preis | `importer`, `tasks`, `catalog` |
| HTTP-API für Tasks, Importläufe, Produkte, Kalkulationsregeln und Suche in Lieferantenartikeln | `api` |
| Suche und Filter: Aufgaben nach Status, Kategorie und Text; Produkte nach Status, „ohne Preis“ und Text; Suche in Importen und Kalkulation; übergreifende Suche über Aufgaben, Produkte und Lieferantenartikel | `api`, `web` |
| Import-Befehl `velopim import <quelle> <datei>` | `cmd/velopim` |
| Oberfläche: Aufgaben mit Freigeben/Ablehnen; im Modell-Task Varianten mit EK, UVP und Verkaufspreis sowie Wahl der Trennmerkmale und Korrektur der Preise vor der Freigabe; erledigte Aufgaben als Umschalter im Reiter Aufgaben; Produktliste je Modell (die Trennung für den Shop ist nur ein Hinweis); Importläufe; Kalkulationsregeln | `web` |

Die Oberfläche wurde im Browser mit den erfundenen Beispieldaten geprüft, nicht mit Hartje-Daten.

## Noch nicht begonnen
- Übernahme freigegebener Änderungen in den Katalog: Verfügbarkeit, Texte, Bilder, UVP, Deaktivierung (bisher wirken nur neue Modelle und EK-Änderungen)
- Auswahl „in den Store“ und Bearbeiten bestehender Produkte (Preis, Texte, Trennmerkmale ändern)
- Veröffentlichen-Task ([25](decisions/0025-publish-task.md))
- Nutzungsrichtlinie in der Pipeline
- Shopify-API-Client mit Dry-Run
- Produktansicht mit Feldherkunft und internen Notizen (die Produktliste zeigt bisher nur Varianten und Preise)
- Pflege von Regeln und Quellen in der Oberfläche (Regeln stehen bisher nur in der Datenbank)
- River-Jobs
- Hartje-Konnektor (Format unbekannt)

## MVP-Umfang
Hartje-Konnektor über Dateiimport mit Rohdatenablage · Normalisierung und Variantengruppierung · Nutzungsrichtlinie · Preiskalkulation · Regel-Engine (Neuprodukt, Preis, Statuswechsel Verfügbarkeit) · Taskliste mit Freigabe und Ablehnung · Produktansicht mit Feldherkunft und internen Notizen · Auswahl „in den Store“ · Shopify-Sync (Produkt, Varianten, Preise, Bilder, Lieferzeit-Metafeld) im Dry-Run und gegen den Development-Store.

## Später
Anmeldung über OIDC und Zuweisung von Aufgaben an Gruppen (z. B. Importprobleme an den Betreiber, Preisänderungen an den Einkauf) · Matching der Bestandsprodukte · Aktionen mit Zeitraum · Eigenbestand · Drift-Erkennung · zweiter Lieferant · Task-Adapter (Todoist usw.) · Benachrichtigungen · weitere Kalkulationsverfahren („UVP übernehmen“) · automatische Preisübernahme je Regel.
