# Stand

## Fertig und getestet
| Baustein | Paket |
|---|---|
| Regel-Engine | `rules` |
| Preiskalkulation (Aufschlag, Zielmarge, Rundung, UVP-Deckel) | `pricing` |
| Verfügbarkeitsmodell mit Mapping | `availability` |
| CSV-Konnektor mit UTF-8 und Windows-1252 (erkannt oder eingestellt) | `connector/csvfile` |
| Änderungserkennung | `actions` |
| Import-Pipeline mit Rohdatenablage, Aktionen und Tasks; neue Artikel als ein Task je Modell; angehaltene Lieferung lässt sich bestätigen; Sammel-Task für Datenprobleme wird nicht wiederholt | `importer` |
| Task-Entscheidung | `tasks` |
| Gruppierung zu Produkten mit Trennmerkmalen | `catalog` |
| Eigener Katalog: Freigabe eines Modells legt Produkte (Entwurf) und Varianten mit eigener Artikelnummer und kalkuliertem Preis an | `catalog`, `tasks` |
| Shopify-Projektion und Diff | `shopify` |
| Betragsgleiche Preisänderung: EK-Änderung schlägt denselben Betrag beim Verkaufspreis vor; Freigabe oder Regel ändert den Preis | `importer`, `tasks`, `catalog` |
| Übernahme von Beschreibung, Bildern, UVP und EAN in den Katalog bei Freigabe oder per Regel | `catalog` |
| Regelpflege in der Oberfläche: anlegen, ändern, ein- und ausschalten, löschen; Vorlagen für häufige Regeln | `api`, `web` |
| Produktansicht je Modell: Name, Beschreibung und Preise bearbeiten, Trennung im Shop ändern (solange nicht in Shopify), „in den Store“ auswählen, interne Notizen, Herkunft und Verlauf jedes Feldes | `api`, `web` |
| Lieferanten anlegen und einrichten: Spaltenzuordnung, Trennzeichen, Zeichensatz, Verfügbarkeitsregeln; Vorschau einer Beispieldatei zum Erkennen der Spalten | `api`, `web` |
| Nutzungsrichtlinie je Lieferant, Marke und Datengruppe, gepflegt im Reiter „Lieferanten“ | `catalog`, `importer`, `api`, `web` |
| Shopify-Sync: Client (Token-Tausch, Wiederholungen bei Drosselung, Schreibsperre außerhalb von Development-Stores), Übertragung je Produkt mit `productSet`, Bilder werden behalten statt neu geladen, Veröffentlichen-Task, Fehler-Task; `velopim sync` und Knopf im Reiter „Shopify“. Gegen den Development-Store `fahrrad-ass-dev` geprüft | `shopify`, `shopsync`, `api`, `web` |
| Shopify-Vorschau (Dry-Run): Projektion aus dem Katalog, Vergleich mit dem letzten Sync-Stand, geplante Änderungen und Ablehnungsgründe je ausgewähltem Produkt | `shopify`, `api`, `web` |
| HTTP-API für Tasks, Importläufe, Produkte, Kalkulationsregeln und Suche in Lieferantenartikeln | `api` |
| Suche und Filter: Aufgaben nach Status, Kategorie und Text; Produkte nach Status, „ohne Preis“ und Text; Suche in Importen und Kalkulation; übergreifende Suche über Aufgaben, Produkte und Lieferantenartikel | `api`, `web` |
| Import per Befehl (`velopim import <quelle> <datei>`) und per Datei-Upload im Reiter „Importe“ | `cmd/velopim`, `api`, `web` |
| Oberfläche: Aufgaben mit Freigeben/Ablehnen; im Modell-Task Varianten mit EK, UVP und Verkaufspreis sowie Wahl der Trennmerkmale und Korrektur der Preise vor der Freigabe; erledigte Aufgaben als Umschalter im Reiter Aufgaben; Produktliste je Modell (die Trennung für den Shop ist nur ein Hinweis); Importläufe; Kalkulationsregeln | `web` |

Die Oberfläche wurde im Browser mit den erfundenen Beispieldaten geprüft, nicht mit Hartje-Daten.

## Noch nicht begonnen
- Bilder und Varianten von Hand bearbeiten; Trennung eines Modells ändern, das schon in Shopify ist
- Sync unmittelbar nach Import oder Freigabe (bisher von Hand oder im festen Abstand)
- Veröffentlichung im Vertriebskanal „Onlineshop“ (braucht das Recht `write_publications`)
- Matching bestehender Shopify-Produkte
- Formular für die Verfügbarkeitsregeln (bisher als JSON-Text im Reiter „Lieferanten“)
- Andere Formate als CSV (BMEcat, veloconnect, API)
- Hintergrundjobs (River)
- Hartje einrichten (Format unbekannt). Liefert Hartje eine CSV-Datei, geht das ohne Code im Reiter „Lieferanten“

## MVP-Umfang
Hartje-Konnektor über Dateiimport mit Rohdatenablage · Normalisierung und Variantengruppierung · Nutzungsrichtlinie · Preiskalkulation · Regel-Engine (Neuprodukt, Preis, Statuswechsel Verfügbarkeit) · Taskliste mit Freigabe und Ablehnung · Produktansicht mit Feldherkunft und internen Notizen · Auswahl „in den Store“ · Shopify-Sync (Produkt, Varianten, Preise, Bilder, Lieferzeit-Metafeld) im Dry-Run und gegen den Development-Store.

## Später
Anmeldung über OIDC und Zuweisung von Aufgaben an Gruppen (z. B. Importprobleme an den Betreiber, Preisänderungen an den Einkauf) · Matching der Bestandsprodukte · Aktionen mit Zeitraum · Eigenbestand · Drift-Erkennung · zweiter Lieferant · Task-Adapter (Todoist usw.) · Benachrichtigungen · weitere Kalkulationsverfahren („UVP übernehmen“) · automatische Preisübernahme je Regel.
