# Offene Punkte

## Vom Betreiber zu klären
- **Hartje-Zugang und Datenformat:** unbekannt (CSV- oder BMEcat-Feed, veloconnect, API?). Keine Annahmen treffen. Bei einer CSV-Datei: im Reiter „Lieferanten“ einen Lieferanten anlegen, die Datei als Beispiel wählen und die Spalten zuordnen. Bei einem anderen Format braucht es einen neuen Konnektor.
- **Varianten bei Hartje:** flach oder Eltern-Kind? Sind Modellname, Farbe und Rahmenform sauber genug für die Gruppierung?
- **Development-Store:** eingerichtet (`fahrrad-ass-dev`), Zugangsdaten stehen in der `.env`.
- **Livegang:** Der Client schreibt nur in Development-Stores ([29](decisions/0029-shopify-sync.md)). Wann und wie soll der Live-Shop freigeschaltet werden, und wie werden die dort schon vorhandenen Produkte zugeordnet ([11](decisions/0011-staged-matching.md))?
- **Lieferzeit im Theme:** Namespace und Key des Metafelds. Vorläufig schreibt velopim `velopim.delivery_time` an die Variante.
- **Kalkulation:** Die Werte pflegt der Betreiber unter „Kalkulation“; festgelegt hat er noch keine. In der lokalen Datenbank steht eine Testregel (Faktor 1,6, ,99, höchstens UVP), die keine Vorgabe ist.
- **Bilder und Texte von Hartje:** Darf der Shop sie verwenden? Bis zur Klärung werden sie übernommen. Abstellen lässt es sich im Reiter „Lieferanten“ ([28](decisions/0028-usage-policy.md)).
- **Preise folgen dem EK ([26](decisions/0026-price-follow-rules.md)):** „Nicht mitgehen, weil noch Lagerbestand da ist“ geht erst, wenn Eigenbestand erfasst wird.
- **Preisregel ([18](decisions/0018-price-changes-need-approval.md)):** Der Betrag wird eins zu eins auf den Ladenpreis übertragen (Annahme), nicht brutto umgerechnet.
- **OIDC und Gruppen:** Anbieter ist Authentik (Vorgabe des Betreibers). Offen: Adresse des Authentik-Servers, Client ID und Secret für velopim, Namen der Gruppen, und ob Gruppen Aufgaben nur filtern oder auch Rechte begrenzen. Ersetzt dann „kein Login“ ([20](decisions/0020-no-login-localhost.md)) und die Annahme „ein Nutzer“.
- **Eigenbestand:** wird nicht geführt (Betreiber: „den Bestand würde keiner pflegen“). Es bleibt bei [9](decisions/0009-no-stock-quantities-mvp.md): kaufbar ist, was der Lieferant liefern kann. Folge: Ein Rad, das im Laden steht, beim Lieferanten aber ausverkauft ist, ist im Shop nicht kaufbar; die Regel „EK-Erhöhung nicht weitergeben, solange Bestand da ist“ gibt es nicht, dafür bleibt die Freigabe von Hand.
- **Schutzregeln ([13](decisions/0013-guard-rules.md)):** vom Betreiber nicht ausdrücklich bestätigt.
- **Vorläufige Entscheidungen ohne Betreiber:** handgepflegte API-Typen ([19](decisions/0019-handwritten-api-types.md)), kein Login ([20](decisions/0020-no-login-localhost.md)), Verfügbarkeit wird live gelesen ([27](decisions/0027-availability-is-live.md)), Stufen der Nutzungsrichtlinie ([28](decisions/0028-usage-policy.md)), Art des Shopify-Syncs ([29](decisions/0029-shopify-sync.md)).

## Bekannte Schwächen
- **Viele Tasks beim Erstimport:** Neue Artikel kommen je Modell als ein Task ([22](decisions/0022-new-model-one-task.md)), ein ganzer Katalog ergibt trotzdem hunderte. Die Taskliste zeigt höchstens 200 Einträge; Filter und Sammelfreigabe fehlen.
- **Modellerkennung:** exakt gleiche Marke, Bezeichnung und Modelljahr. Steckt die Farbe in der Bezeichnung, zerfällt ein Modell.
- **Freigaben ohne Wirkung:** Aufgaben zu Verfügbarkeit und entfallenen Artikeln sind Hinweise ([27](decisions/0027-availability-is-live.md)). Änderungen an Bezeichnung, Farbe, Rahmenhöhe und Merkmalen werden nur protokolliert.
- **Alte Preis-Tasks:** Eine EK-Änderung, die vor dem Anlegen des Produkts kam, hat keinen Preisvorschlag; ihre Freigabe ändert nichts.
- **Nachgelieferte Varianten:** Sie kommen zum bestehenden Produkt, ohne zu prüfen, ob dessen Optionen passen oder ob es die Kombination dort schon gibt.
- **Fehlende Optionswerte:** Eine Variante ohne Farbe oder Rahmenhöhe wird angelegt; erst die Shopify-Prüfung würde sie ablehnen.
- **Produktdaten vom ersten Artikel:** Kategorie und Beschreibung des Produkts stammen vom ersten Artikel der Gruppe.
- **Handänderungen und Lieferant:** Ein von Hand geänderter Text wird überschrieben, wenn später eine Textänderung des Lieferanten freigegeben wird. Es gibt noch keine Sperre je Feld.
- **Alte englische Texte:** Datenprobleme aus Importen vor der Umstellung stehen noch englisch in der Datenbank.
- **Gruppierung vergleicht exakt:** „Schwarz“ und „schwarz“ sind zwei Werte.
- **Lieferzeit-Texte:** stehen fest im Code (`availability.Derive`), sollen konfigurierbar werden.
- **Testbilder:** Die Beispieldaten haben unerreichbare Bildadressen (`example.test`); ein Sync damit erzeugt den Task „Bilder fehlen in Shopify“.
- **Automatischer Sync braucht den Schreibschalter:** Solange `SHOPIFY_WRITE_ENABLED=false` in der `.env` steht, wird nichts automatisch übertragen.
- **Abholen nur als einzelne Datei:** Eine API mit mehreren Aufrufen oder Seiten wird noch nicht unterstützt ([30](decisions/0030-fetch-jobs.md)).
- **Mail-Einstellungen:** SMTP-Server, Absender und Empfänger fehlen noch in der `.env`; bis dahin wird keine Mail gesendet. Gegen einen echten Mailserver ist der Versand nicht geprüft, nur gegen einen lokalen Testserver.
- **Jobs ohne Ansicht:** Wartende und gelaufene Jobs sind in der Oberfläche nicht sichtbar, nur ihr Ergebnis (Importlauf, letzter Abruf, Aufgabe bei Fehler).
- **Shopify-Grenzen:** 3 Optionen und 2048 Varianten je Produkt stehen als Konstanten im Code und sind gegen die verwendete API-Version zu prüfen.
- **Testdaten:** `testdata/example_supplier.csv` ist erfunden und kein Hartje-Format.
