# Offene Punkte

## Vom Betreiber zu klären
- **Hartje-Zugang und Datenformat:** unbekannt (CSV- oder BMEcat-Feed, veloconnect, API?). Keine Annahmen treffen; bis zur Klärung gilt die Dateischnittstelle mit Testdaten. Am meisten hilft eine echte Beispieldatei.
- **Varianten bei Hartje:** flach oder Eltern-Kind? Sind Modellname, Farbe und Rahmenform sauber genug für die Gruppierung?
- **Development-Store und API-Token:** fehlen. Das Token kommt nur als Umgebungsvariable, nie in den Chat oder ins Repository.
- **Lieferzeit im Theme:** Namespace und Key des Metafelds.
- **Kalkulation:** Die Werte pflegt der Betreiber unter „Kalkulation“; festgelegt hat er noch keine. In der lokalen Datenbank steht eine Testregel (Faktor 1,6, ,99, höchstens UVP), die keine Vorgabe ist.
- **Bilder und Texte von Hartje:** Darf der Shop sie verwenden? Bis zur Klärung werden sie übernommen. Abstellen lässt es sich im Reiter „Lieferanten“ ([28](decisions/0028-usage-policy.md)).
- **Preise folgen dem EK ([26](decisions/0026-price-follow-rules.md)):** „Nicht mitgehen, weil noch Lagerbestand da ist“ geht erst, wenn Eigenbestand erfasst wird.
- **Preisregel ([18](decisions/0018-price-changes-need-approval.md)):** Der Betrag wird eins zu eins auf den Ladenpreis übertragen (Annahme), nicht brutto umgerechnet.
- **OIDC und Gruppen:** Welcher Anbieter? Sollen Gruppen Aufgaben nur filtern oder auch Rechte begrenzen? Ersetzt dann „kein Login“ ([20](decisions/0020-no-login-localhost.md)) und die Annahme „ein Nutzer“.
- **Schutzregeln ([13](decisions/0013-guard-rules.md)):** vom Betreiber nicht ausdrücklich bestätigt.
- **Vorläufige Entscheidungen ohne Betreiber:** handgepflegte API-Typen ([19](decisions/0019-handwritten-api-types.md)), kein Login ([20](decisions/0020-no-login-localhost.md)), Verfügbarkeit wird live gelesen ([27](decisions/0027-availability-is-live.md)).

## Bekannte Schwächen
- **Viele Tasks beim Erstimport:** Neue Artikel kommen je Modell als ein Task ([22](decisions/0022-new-model-one-task.md)), ein ganzer Katalog ergibt trotzdem hunderte. Die Taskliste zeigt höchstens 200 Einträge; Filter und Sammelfreigabe fehlen.
- **Modellerkennung:** exakt gleiche Marke, Bezeichnung und Modelljahr. Steckt die Farbe in der Bezeichnung, zerfällt ein Modell.
- **Doppelte Sammel-Tasks:** Jeder Import mit Datenproblemen legt einen neuen Task an, auch bei denselben Problemen wie im Lauf davor.
- **Freigaben ohne Wirkung:** Aufgaben zu Verfügbarkeit und entfallenen Artikeln sind Hinweise ([27](decisions/0027-availability-is-live.md)). Änderungen an Bezeichnung, Farbe, Rahmenhöhe und Merkmalen werden nur protokolliert.
- **Alte Preis-Tasks:** Eine EK-Änderung, die vor dem Anlegen des Produkts kam, hat keinen Preisvorschlag; ihre Freigabe ändert nichts.
- **Nachgelieferte Varianten:** Sie kommen zum bestehenden Produkt, ohne zu prüfen, ob dessen Optionen passen oder ob es die Kombination dort schon gibt.
- **Fehlende Optionswerte:** Eine Variante ohne Farbe oder Rahmenhöhe wird angelegt; erst die Shopify-Prüfung würde sie ablehnen.
- **Produktdaten vom ersten Artikel:** Kategorie und Beschreibung des Produkts stammen vom ersten Artikel der Gruppe.
- **Handänderungen und Lieferant:** Ein von Hand geänderter Text wird überschrieben, wenn später eine Textänderung des Lieferanten freigegeben wird. Es gibt noch keine Sperre je Feld.
- **Angehaltener Import:** Es gibt noch keinen Weg, einen angehaltenen Lauf zu bestätigen.
- **Englische Texte:** Meldungen zu Datenproblemen und Regelnamen sind englisch; die Oberfläche übersetzt nur die Entscheidungsgründe.
- **Gruppierung vergleicht exakt:** „Schwarz“ und „schwarz“ sind zwei Werte.
- **Zeichensatz:** Der CSV-Konnektor erwartet UTF-8; Windows-1252 wird nicht unterstützt.
- **Lieferzeit-Texte:** stehen fest im Code (`availability.Derive`), sollen konfigurierbar werden.
- **Shopify-Grenzen:** 3 Optionen und 2048 Varianten je Produkt stehen als Konstanten im Code und sind gegen die verwendete API-Version zu prüfen.
- **Testdaten:** `testdata/example_supplier.csv` ist erfunden und kein Hartje-Format.
