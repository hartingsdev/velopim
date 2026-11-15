# Stand

## Fertig und getestet
| Baustein | Paket |
|---|---|
| Regel-Engine | `rules` |
| Preiskalkulation (Aufschlag, Zielmarge, Rundung, UVP-Deckel) | `pricing` |
| Verfügbarkeitsmodell mit Mapping | `availability` |
| CSV-Konnektor | `connector/csvfile` |
| Änderungserkennung | `actions` |
| Import-Pipeline mit Rohdatenablage, Aktionen und Tasks | `importer` |
| Task-Entscheidung | `tasks` |
| Gruppierung zu Produkten mit Trennmerkmalen (reine Funktion, nicht angebunden) | `catalog` |
| Shopify-Projektion und Diff | `shopify` |
| HTTP-API für Tasks und Importläufe | `api` |
| Import-Befehl `velopim import <quelle> <datei>` | `cmd/velopim` |
| Oberfläche: offene Aufgaben mit Freigeben/Ablehnen, erledigte Aufgaben, Importläufe | `web` |

Die Oberfläche wurde im Browser mit den erfundenen Beispieldaten geprüft, nicht mit Hartje-Daten.

## Noch nicht begonnen
- Eigener Katalog in der Datenbank: Produkte und Varianten aus der Gruppierung anlegen, Trennmerkmale speichern
- Übernahme freigegebener Aktionen in den Katalog (eine Freigabe wird bisher nur protokolliert)
- Betragsgleiche Preissenkung als Vorschlag
- Nutzungsrichtlinie in der Pipeline
- Shopify-API-Client mit Dry-Run
- Produktansicht mit Feldherkunft und internen Notizen
- Pflege von Regeln, Quellen und Kalkulation in der Oberfläche
- River-Jobs
- Hartje-Konnektor (Format unbekannt)

## MVP-Umfang
Hartje-Konnektor über Dateiimport mit Rohdatenablage · Normalisierung und Variantengruppierung · Nutzungsrichtlinie · Preiskalkulation · Regel-Engine (Neuprodukt, Preis, Statuswechsel Verfügbarkeit) · Taskliste mit Freigabe und Ablehnung · Produktansicht mit Feldherkunft und internen Notizen · Auswahl „in den Store“ · Shopify-Sync (Produkt, Varianten, Preise, Bilder, Lieferzeit-Metafeld) im Dry-Run und gegen den Development-Store.

## Später
Matching der Bestandsprodukte · Aktionen mit Zeitraum · Eigenbestand · Drift-Erkennung · zweiter Lieferant · Task-Adapter (Todoist usw.) · Benachrichtigungen · weitere Kalkulationsverfahren („UVP übernehmen“) · automatische Preisübernahme je Regel.
