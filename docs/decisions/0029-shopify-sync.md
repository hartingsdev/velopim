# 29. Shopify-Sync über productSet, Verkaufbarkeit über Bestandsführung

**Status:** Vorläufig, ohne Betreiber entschieden; gegen den Development-Store geprüft

## Kontext
Die Projektion ([6](0006-shopify-projection-allowlist.md)) muss zu Shopify, und zwar so, dass ein wiederholter Lauf nichts doppelt anlegt. Stückzahlen führt velopim nicht ([9](0009-no-stock-quantities-mvp.md)), ein nicht lieferbares Rad darf aber nicht kaufbar sein.

## Entscheidung
- **Mutation:** `productSet` der Admin GraphQL API (Version `2026-10`), synchron. Sie beschreibt das ganze Produkt: Varianten und Bilder, die im Aufruf fehlen, entfernt Shopify. Ein neues Produkt wird ohne ID gesendet, danach immer mit der gespeicherten Shopify-ID. Derselbe Aufruf zweimal ändert nichts.
- **Wann gesendet wird:** nur wenn sich die Projektion vom zuletzt bestätigten Stand unterscheidet (`shopify.Diff`). Der bestätigte Stand steht in `shopify_listing.synced_projection`.
- **Verkaufbar:** Eine verkaufbare Variante hat keine Bestandsführung (`tracked: false`) und ist immer kaufbar. Eine nicht verkaufbare Variante hat Bestandsführung mit Menge 0 am ersten Standort und „nicht überverkaufen“; Shopify zeigt sie als ausverkauft.
- **Lieferzeit:** Text als Metafeld der Variante, vorläufig `velopim.delivery_time`.
- **Status:** Entwurf, bis der Veröffentlichen-Task freigegeben ist ([25](0025-publish-task.md)), dann aktiv. Ein Produkt, das aus dem Store genommen wird, geht in Shopify zurück auf Entwurf; gelöscht wird nichts.
- **Fehler:** Je Produkt eigenständig. Ein Fehler steht am Listing und wird ein Task (höchstens ein offener je Produkt); der nächste erfolgreiche Lauf schließt ihn.
- **Auslöser:** von Hand über den Reiter „Shopify“ oder `velopim sync`; wahlweise regelmäßig, solange der Server läuft (`VELOPIM_SYNC_INTERVAL=15m`, standardmäßig aus). Eine Job-Queue gibt es noch nicht.
- **Bilder:** Shopify lädt Bilder nach dem Aufruf selbst. Der Sync wartet bis zu 30 Sekunden darauf, merkt sich je Bildadresse die Shopify-ID und sendet bei späteren Läufen diese ID, damit Shopify das Bild behält, statt es neu zu laden. Kann Shopify ein Bild nicht laden, entsteht ein Task; das Bild wird beim nächsten Lauf erneut gesendet.
- **Schutz:** Der Client schreibt nur mit `SHOPIFY_WRITE_ENABLED=true` und nur, wenn Shopify den Store als Development-Store meldet und dessen Adresse der konfigurierten entspricht.

## Alternativen
- **Einzelne Mutationen je Änderung** (`productUpdate`, `productVariantsBulkUpdate` …): feiner, aber viele Aufrufe und Zwischenzustände. `productSet` passt zum Modell „PIM ist führend“.
- **Lieferantenbestand als Menge schreiben:** widerspricht [9](0009-no-stock-quantities-mvp.md).

## Folgen
- `productSet` überschreibt die gesendeten Felder. Was Apps oder Menschen in Shopify an Titel, Beschreibung, Preisen, Varianten oder Bildern ändern, geht beim nächsten Lauf verloren ([7](0007-field-ownership.md)). Tags, SEO, Sammlungen und Metafelder anderer Namensräume sendet velopim nicht und lässt sie stehen.
- **Der Schutz „nur Development-Store“ verhindert den Betrieb gegen den Live-Shop.** Das ist gewollt, bis der Betreiber den Schritt entscheidet; dann braucht es eine bewusste Freischaltung.
- „Aktiv“ heißt noch nicht „im Onlineshop sichtbar“: Die Veröffentlichung in einem Vertriebskanal braucht das Recht `write_publications`, das die App nicht hat. Im Development-Store ist das Produkt nach der Freigabe aktiv, aber unter Umständen keinem Kanal zugeordnet.
- Mehr als 250 Varianten je Produkt liest die Rückmeldung nicht vollständig.
- Ein nicht ladbares Bild wird erst beim nächsten Lauf mit einer Änderung am Produkt erneut versucht.
