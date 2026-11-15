# 21. Produktschnitt je Modell einstellbar

**Status:** Angenommen (Vorgabe des Betreibers), umgesetzt: Die Trennmerkmale werden bei der Freigabe des Modells gewählt. Sie lassen sich in der Produktansicht ändern, solange kein Produkt des Modells in Shopify ist

## Kontext
Zu klären war, was eine Produktseite im Shop ist: ein Modell mit allen Farben oder eine Seite je Farbe. Der Betreiber wollte das je Artikel entscheiden können und nicht auf Farbe beschränken.

## Entscheidung
Standard: ein Produkt je Modell (Marke, Bezeichnung, Modelljahr) mit Farbe und Rahmenhöhe als Optionen. Je Modell kann eine Liste von Trennmerkmalen gesetzt werden: Farbe, Rahmenhöhe oder ein beliebiges Lieferantenattribut (z. B. Rahmenform, Antrieb), auch mehrere. Jede Wertekombination wird ein eigenes Produkt, der Wert kommt in den Titel. Die übrigen Merkmale werden Optionen; Merkmale ohne Werte fallen weg. Umsetzung: `catalog.Group`.

## Alternativen
- **Fest pro Modell** oder **fest pro Farbe:** einfacher, passt aber nicht für alle Räder.
- **Nur Schalter „nach Farbe trennen“:** deckt Rahmenform und Antrieb nicht ab, die im Fahrradhandel oft die sinnvollere Trennung sind.

## Folgen
Die Gruppierung meldet nicht unterscheidbare Artikel, fehlende Werte und mehr als drei Optionen (Shopify-Grenze). Werte werden exakt verglichen. Beim Ändern werden die Produkte des Modells neu angelegt; Varianten behalten Artikelnummer, Preis und Verlauf, Notizen und Aufgaben wandern zum ersten neuen Produkt, die Auswahl für den Store entfällt. Ist ein Produkt des Modells schon in Shopify, wird das Ändern abgelehnt, weil es dort neue Produkte mit neuen Adressen bedeuten würde. Ein uneinheitlicher Schnitt macht Übersicht und Filter im Shop uneinheitlich.
