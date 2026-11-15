# 25. Neue Produkte gehen als Entwurf zu Shopify, Veröffentlichen ist ein Task

**Status:** Angenommen (Vorgabe des Betreibers), noch nicht umgesetzt

## Kontext
Ein Produkt soll nicht ungeprüft im Shop sichtbar werden.

## Entscheidung
Der Sync legt ein neues Produkt in Shopify als Entwurf an. Dazu entsteht ein Task „Veröffentlichen“ mit der Aufforderung, das Produkt im Shop zu prüfen, und dem Link dorthin. Erst das Erledigen des Tasks setzt das Produkt aktiv.

## Alternativen
- **Sofort aktiv:** Fehler in Bildern, Texten oder Preisen wären öffentlich.
- **Entwurf ohne Task:** Entwürfe bleiben liegen.

## Folgen
Braucht den Shopify-Sync und den Development-Store. Der Status (Entwurf/aktiv) wird damit ein Feld, das velopim besitzt ([7](0007-field-ownership.md)).
