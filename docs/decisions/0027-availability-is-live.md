# 27. Verfügbarkeit wird nicht kopiert, sondern live gelesen

**Status:** Vorläufig, ohne Betreiber entschieden

## Kontext
Freigegebene Änderungen des Lieferanten werden in den eigenen Katalog übernommen ([12](0012-supplier-view-always-current.md)). Für Verfügbarkeit und für Artikel, die aus dem Feed fallen, stellt sich die Frage, ob der Katalog eine eigene, freigegebene Kopie halten soll.

## Entscheidung
Der Katalog speichert keine Verfügbarkeit. Die Shopify-Projektion liest sie vom bevorzugten Lieferantenartikel der Variante; ein Artikel, der nicht mehr im Feed ist, gilt als nicht verkaufbar. Aufgaben zu Verfügbarkeit und entfallenen Artikeln sind damit Hinweise: Ihre Freigabe ändert nichts, die Ablehnung auch nicht.

Übernommen werden bei Freigabe oder per Regel: Beschreibung, Bilder, UVP (als Streichpreis) und EAN. Der Einkaufspreis ändert den Verkaufspreis um denselben Betrag ([18](0018-price-changes-need-approval.md)). Bezeichnung, Farbe, Rahmenhöhe und weitere Merkmale bestimmen, zu welchem Produkt und welcher Variante ein Artikel gehört; ihre Änderung wird nicht automatisch übernommen.

## Alternativen
- **Freigegebene Kopie der Verfügbarkeit:** Ein ausverkauftes Rad bliebe verkaufbar, bis jemand den Task freigibt. Das Risiko, etwas zu verkaufen, das nicht lieferbar ist, wiegt schwerer als die Kontrolle.

## Folgen
- Für Verfügbarkeit lohnt eine Regel „automatisch“, damit keine Hinweis-Tasks entstehen. Angelegt ist sie nicht; das entscheidet der Betreiber.
- Soll ein Rad trotz Lieferantenstatus verkaufbar bleiben (eigener Lagerbestand), braucht es den Eigenbestand ([9](0009-no-stock-quantities-mvp.md)).
- Ändert der Lieferant Farbe oder Rahmenhöhe eines Artikels, stimmt die Variante nicht mehr; das fällt bisher nur über den Task auf.
