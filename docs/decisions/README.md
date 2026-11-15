# Entscheidungen

Jede Architekturentscheidung hat eine eigene Datei mit Kontext, Entscheidung, Alternativen und Folgen. Neue Entscheidungen bekommen die nächste Nummer; eine überholte Entscheidung wird nicht gelöscht, sondern bekommt den Status „Ersetzt durch …“.

| Nr. | Entscheidung | Status |
|---|---|---|
| 1 | [Go als Backend-Sprache](0001-go-backend.md) | Angenommen (Betreiber: kein TypeScript/Node im Backend, „eher Richtung Go, Rust“) |
| 2 | [PostgreSQL als Datenbank](0002-postgresql.md) | Angenommen (vom Betreiber bestätigt) |
| 3 | [React-Frontend, eingebettet ins Binary](0003-react-frontend.md) | Angenommen (Wunsch des Betreibers) |
| 4 | [Feldherkunft: typisierte Spalten plus Änderungsprotokoll](0004-provenance-field-change.md) | Angenommen (Betreiber war unsicher, hat dem Vorschlag gefolgt) |
| 5 | [Lieferantenartikel getrennt von Produkt und Variante](0005-supplier-item-separate.md) | Angenommen |
| 6 | [Shopify-Projektion als Positivliste](0006-shopify-projection-allowlist.md) | Angenommen |
| 7 | [Feldhoheit statt „nie in Shopify bearbeiten“](0007-field-ownership.md) | Angenommen |
| 8 | [Verfügbarkeit: nur Statuswechsel sind Aktionen](0008-availability-status-actions.md) | Angenommen |
| 9 | [Bestand im MVP: nur „verkaufbar“ und Lieferzeit](0009-no-stock-quantities-mvp.md) | Angenommen |
| 10 | [Regeln strukturiert statt freier Regelsprache](0010-structured-rules.md) | Angenommen |
| 11 | [Matching gestuft: EAN, dann Herstellernummer, dann manuell](0011-staged-matching.md) | Angenommen, noch nicht gebaut |
| 12 | [Lieferantensicht zeigt immer den letzten Stand](0012-supplier-view-always-current.md) | Angenommen (vom Betreiber bestätigt) |
| 13 | [Schutzregeln erzwingen Freigabe](0013-guard-rules.md) | Angenommen, vom Betreiber nicht ausdrücklich bestätigt |
| 14 | [Datenbankzugriff über sqlc](0014-sqlc.md) | Angenommen (Wunsch des Betreibers) |
| 15 | [Import anhalten, wenn mehr als 20 % der Artikel fehlen](0015-hold-import-on-drop.md) | Angenommen |
| 16 | [Eine neuere offene Aktion ersetzt die ältere](0016-supersede-pending-actions.md) | Angenommen |
| 17 | [Datenprobleme eines Imports ergeben einen Sammel-Task](0017-collective-data-issue-task.md) | Angenommen |
| 18 | [Preisänderungen immer zur Freigabe, Senkungen betragsgleich](0018-price-changes-need-approval.md) | Angenommen (Vorgabe des Betreibers), umgesetzt |
| 19 | [API-Typen von Hand gepflegt](0019-handwritten-api-types.md) | Vorläufig, ohne Betreiber entschieden |
| 20 | [Kein Login, nur localhost](0020-no-login-localhost.md) | Vorläufig, ohne Betreiber entschieden |
| 21 | [Produktschnitt je Modell einstellbar](0021-configurable-product-split.md) | Angenommen (Vorgabe des Betreibers), umgesetzt |
| 22 | [Ein neues Modell ist eine Aktion und ein Task](0022-new-model-one-task.md) | Angenommen (vom Betreiber bestätigt), umgesetzt |
| 23 | [Eigene Artikelnummern für Varianten](0023-own-skus.md) | Angenommen (Vorgabe des Betreibers), umgesetzt |
| 24 | [Kalkulation in der Oberfläche pflegen, Preise im Freigabe-Task zeigen](0024-pricing-config-in-ui.md) | Angenommen (Vorgabe des Betreibers) |
| 25 | [Neue Produkte gehen als Entwurf zu Shopify, Veröffentlichen ist ein Task](0025-publish-task.md) | Angenommen (Vorgabe des Betreibers), umgesetzt |
| 26 | [Regeln entscheiden, ob Preise dem Einkaufspreis folgen](0026-price-follow-rules.md) | Angenommen (Vorgabe des Betreibers), umgesetzt |
| 27 | [Verfügbarkeit wird nicht kopiert, sondern live gelesen](0027-availability-is-live.md) | Vorläufig, ohne Betreiber entschieden |
| 28 | [Nutzungsrichtlinie je Lieferant, Marke und Datengruppe](0028-usage-policy.md) | Angenommen, umgesetzt; Bedeutung der Stufen vorläufig |
| 29 | [Shopify-Sync über productSet, Verkaufbarkeit über Bestandsführung](0029-shopify-sync.md) | Vorläufig, gegen den Development-Store geprüft |
| 30 | [velopim holt Lieferantendateien selbst ab, über Jobs in River](0030-fetch-jobs.md) | Angenommen (Vorgabe des Betreibers), umgesetzt; Ausgestaltung vorläufig |
| 31 | [Wiedervorlage für Aufgaben und tägliche Mail](0031-snooze-and-daily-mail.md) | Angenommen (Vorgabe des Betreibers), umgesetzt |
| 32 | [Produktbeschreibungen als Vorschlag vom Textmodell](0032-description-drafts.md) | Vorläufig, ohne Betreiber im Detail entschieden; gegen die echte API nicht geprüft |
