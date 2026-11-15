# 21. Produktschnitt je Modell einstellbar

**Status:** Angenommen (Vorgabe des Betreibers), umgesetzt: Die Trennmerkmale werden bei der Freigabe des Modells gewählt. Nachträgliches Ändern fehlt

## Kontext
Zu klären war, was eine Produktseite im Shop ist: ein Modell mit allen Farben oder eine Seite je Farbe. Der Betreiber wollte das je Artikel entscheiden können und nicht auf Farbe beschränken.

## Entscheidung
Standard: ein Produkt je Modell (Marke, Bezeichnung, Modelljahr) mit Farbe und Rahmenhöhe als Optionen. Je Modell kann eine Liste von Trennmerkmalen gesetzt werden: Farbe, Rahmenhöhe oder ein beliebiges Lieferantenattribut (z. B. Rahmenform, Antrieb), auch mehrere. Jede Wertekombination wird ein eigenes Produkt, der Wert kommt in den Titel. Die übrigen Merkmale werden Optionen; Merkmale ohne Werte fallen weg. Umsetzung: `catalog.Group`.

## Alternativen
- **Fest pro Modell** oder **fest pro Farbe:** einfacher, passt aber nicht für alle Räder.
- **Nur Schalter „nach Farbe trennen“:** deckt Rahmenform und Antrieb nicht ab, die im Fahrradhandel oft die sinnvollere Trennung sind.

## Folgen
Die Gruppierung meldet nicht unterscheidbare Artikel, fehlende Werte und mehr als drei Optionen (Shopify-Grenze). Werte werden exakt verglichen. Umstellen nach dem ersten Sync legt Shopify-Produkte neu an (neue URLs) und soll einen Task mit Warnung erzeugen. Ein uneinheitlicher Schnitt macht Übersicht und Filter im Shop uneinheitlich.
