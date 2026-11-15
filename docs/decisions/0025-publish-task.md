# 25. Neue Produkte gehen als Entwurf zu Shopify, Veröffentlichen ist ein Task

**Status:** Angenommen (Vorgabe des Betreibers), umgesetzt

## Kontext
Ein Produkt soll nicht ungeprüft im Shop sichtbar werden.

## Entscheidung
Der Sync legt ein neues Produkt in Shopify als Entwurf an. Dazu entsteht ein Task „Veröffentlichen“ mit der Aufforderung, das Produkt im Shop zu prüfen, und dem Link dorthin. Erst das Erledigen des Tasks setzt das Produkt aktiv.

## Alternativen
- **Sofort aktiv:** Fehler in Bildern, Texten oder Preisen wären öffentlich.
- **Entwurf ohne Task:** Entwürfe bleiben liegen.

## Folgen
Der Task enthält den Link zum Produkt in der Shopify-Verwaltung; von dort führt „Vorschau“ in den Shop. Freigeben setzt das Produkt in velopim aktiv, die nächste Übertragung setzt es in Shopify aktiv. Siehe [29](0029-shopify-sync.md) zur Einschränkung bei den Vertriebskanälen. Der Status (Entwurf/aktiv) wird damit ein Feld, das velopim besitzt ([7](0007-field-ownership.md)).
