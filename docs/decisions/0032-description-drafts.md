# 32. Produktbeschreibungen als Vorschlag vom Textmodell

**Status:** Vorläufig, ohne Betreiber im Detail entschieden (Wunsch des Betreibers: „Generierung der Produktbeschreibung“); gegen die echte API noch nicht geprüft

## Kontext
Lieferantentexte fehlen oft, sind dünn oder dürfen im Shop nicht verwendet werden ([28](0028-usage-policy.md)). Der Betreiber möchte Beschreibungen erzeugen lassen. Ein erzeugter Text kann Falsches behaupten (erfundene Ausstattung) und landet über den Sync im Shop.

## Entscheidung
- **Vorschlag, keine Automatik:** In der Produktansicht füllt „Beschreibung vorschlagen“ das Textfeld. Gespeichert wird nichts, bis der Betreiber den Text gelesen hat und „Speichern“ wählt; erst dann steht er im Katalog (Herkunft „user“) und geht mit dem nächsten Sync zu Shopify. „Vorschlag verwerfen“ stellt den alten Text wieder her.
- **Anbieter:** Claude API über das offizielle Go-SDK. Schlüssel in `ANTHROPIC_API_KEY`; ohne Schlüssel gibt es den Knopf nicht. Modell in `VELOPIM_AI_MODEL`, Vorgabe `claude-opus-5-5`.
- **Was das Modell erfährt** (`describe.Input`, Positivliste wie bei der Shopify-Projektion): Marke, Name, Modelljahr, Kategorie, wählbare Optionen, technische Daten der Lieferantenartikel und den Lieferantentext. Nie: Einkaufspreise, Verkaufspreise, Lieferantenname, Artikelnummern, interne Notizen, Verfügbarkeit.
- **Nutzungsrichtlinie:** Technische Daten gehen an das Modell, solange ihre Gruppe nicht auf „ignorieren“ steht (Tatsachen). Der Lieferantentext nur, wenn Texte auf „verwenden“ stehen: Ein Text, der nicht in den Shop darf, soll auch nicht umformuliert dort landen.
- **Form:** Das Modell antwortet nach einem festen Schema (Absätze und Stichpunkte als reiner Text). velopim baut daraus das HTML und maskiert alles; das Modell kann kein eigenes Markup einschleusen.
- **Auftrag:** nur Tatsachen aus den Angaben, nichts ergänzen, sachlich, keine direkte Anrede, keine Preise, Lieferzeiten, Farben- oder Größenlisten. Der Wortlaut steht in `describe.System`.

## Alternativen
- **Automatisch beim Anlegen eines Produkts erzeugen und speichern:** spart Klicks, aber ungeprüfte Texte kämen in den Shop. Später als Job mit eigener Aufgabe „Text prüfen“ denkbar.
- **HTML direkt vom Modell:** flexibler, braucht aber einen Filter für erlaubte Elemente.
- **Anderer Anbieter oder lokales Modell:** nicht geprüft; die Schnittstelle `api.Describer` ist klein genug für einen Tausch.

## Folgen
- Produktdaten (keine internen, keine personenbezogenen) verlassen das Haus in Richtung Anthropic. Jeder Vorschlag kostet Geld; bei kurzen Texten Bruchteile eines Cents bis wenige Cent, je nach Modell.
- Der Aufruf läuft im Request und dauert einige Sekunden. Für viele Produkte auf einmal wäre ein Job mit der Batch-API (halber Preis) der richtige Weg; das gibt es noch nicht.
- Erzeugt wird nur die Beschreibung. Name, SEO-Felder und Bildtexte kommen nicht vom Modell (siehe offene Punkte).
- Die Prüfung liegt beim Betreiber: Das Modell soll nichts erfinden, garantiert ist das nicht.
