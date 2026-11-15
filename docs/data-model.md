# Datenmodell

Maßgeblich ist `db/migrations/`. Diese Seite erklärt, wozu die Tabellen da sind.

## Quellen und Importe
- **`source`:** ein Lieferant oder Feed. Enthält die Art des Konnektors, dessen Konfiguration (z. B. Spaltenzuordnung der CSV-Datei) und das Mapping der Verfügbarkeit, beides als JSON.
- **`import_run`:** ein Importlauf mit Status (`running`, `done`, `failed`, `held`), Pfad und Hash der unverändert abgelegten Originaldatei und einer Zusammenfassung (neu, geändert, entfallen, Mengenänderungen, Datenprobleme).
- **`raw_record`:** eine Rohversion eines Artikels. Eine neue Zeile entsteht nur bei geändertem Inhalt; Zeilen werden nie geändert oder gelöscht.

## Lieferantensicht
- **`supplier_item`:** normalisierte Sicht je Quelle: Artikelnummer, EAN, Herstellernummer, Marke, Bezeichnung, Modelljahr, Kategorie, Farbe, Rahmenhöhe, Beschreibung, Bilder, EK (netto), UVP (brutto), Attribute, Verfügbarkeit. Zeigt immer den letzten Stand des Lieferanten ([12](decisions/0012-supplier-view-always-current.md)). `active = false`, sobald der Artikel im Feed fehlt.

## Eigener Katalog
- **`product`:** ein Produkt, im Standard je Modell und Modelljahr, mit Verweis auf den Vorgänger. Der Schnitt ist je Modell einstellbar ([21](decisions/0021-configurable-product-split.md)); die Trennmerkmale haben noch keine Spalte.
- **`variant`:** eine Variante mit eigener SKU, Optionswerten, Verkaufs- und Streichpreis.
- **`variant_supplier_item`:** n:m zwischen Variante und Lieferantenartikel, mit einem bevorzugten Lieferanten je Variante ([5](decisions/0005-supplier-item-separate.md)).

Der Katalog ist im Schema angelegt, wird aber noch nicht befüllt.

## Steuerung
- **`usage_policy`:** je Quelle und Marke pro Feldgruppe (Bilder, Texte, Preise, Attribute): `use`, `internal_only` oder `ignore`. Noch nicht in der Pipeline ausgewertet.
- **`pricing_rule`:** Kalkulation je Marke und Kategorie (Verfahren, Wert, MwSt, Rundung, UVP als Obergrenze). Die spezifischste Regel gilt.
- **`rule`:** Regel der Regel-Engine: Geltungsbereich (Quelle, Marke, Kategorie), Aktionstyp, Bedingungen, Ergebnis (`auto` oder `approval`), Schutzregel ja/nein ([10](decisions/0010-structured-rules.md), [13](decisions/0013-guard-rules.md)).

## Aktionen und Tasks
- **`action`:** ein Änderungsvorschlag: Typ, Feld, alter und neuer Wert, die Fakten, auf denen die Regeln entschieden haben, die greifende Regel und der Grund. Status: `pending_approval`, `auto_applied`, `approved`, `rejected`, `superseded`. Eine Aktion `new_product` gilt für ein ganzes Modell (`model_key`) statt für einen einzelnen Artikel ([22](decisions/0022-new-model-one-task.md)).
- **`action_item`:** die Lieferantenartikel, die eine Modell-Aktion umfasst.
- **`task`:** eine Aufgabe für den Betreiber: Art, Titel, Priorität (1 = hoch), Verweis auf Aktion, Artikel oder Importlauf. Status: `open`, `done`, `dismissed`. Das Schließen eines Freigabe-Tasks entscheidet die Aktion dahinter. `external_system` und `external_ref` sind für spätere Task-Adapter vorgesehen.

## Herkunft
- **`field_change`:** Protokoll jeder Änderung eines Feldwerts: Entität, Feld, alt, neu, Herkunft (`import`, `rule`, `user`, `system`) mit Importlauf, Regel, Aktion oder Person, Zeitpunkt, Begründung. Wird nur ergänzt ([4](decisions/0004-provenance-field-change.md)).

## Nur intern
- **`internal_note`**, **`internal_field`:** Notizen und freie interne Felder zu Produkt, Variante, Lieferantenartikel oder Quelle (z. B. Ansprechpartner, Bestellhinweise). Kommen in der Shopify-Projektion nicht vor.

## Shopify
- **`shopify_listing`:** ob ein Produkt in den Store soll, die Shopify-IDs, die zuletzt bestätigte Projektion mit Hash und der letzte Fehler.

## Verfügbarkeit
Lieferantenunabhängiges Modell in `internal/availability`: Status (`in_stock`, `orderable`, `preorder`, `on_request`, `unavailable`, `discontinued`, `unknown`), Lieferzeit in Tagen von/bis, Termin, Menge, Quelle, Stand. Jede Quelle bildet ihre Rohfelder über geordnete Mapping-Regeln darauf ab. Nur ein Statuswechsel ist eine Aktion ([8](decisions/0008-availability-status-actions.md)); zu Shopify gehen nur „verkaufbar“ und ein Lieferzeit-Text ([9](decisions/0009-no-stock-quantities-mvp.md)).

## Preise
Alle Beträge in Cent. Der EK kommt vom Lieferantenartikel. `pricing.Calculate` rechnet per Aufschlag oder Zielmarge, mit Mehrwertsteuer, Rundung und optional der UVP als Obergrenze. Preisänderungen brauchen immer eine Freigabe ([18](decisions/0018-price-changes-need-approval.md)).
