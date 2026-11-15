# Offene Punkte

## Vom Betreiber zu klären
- **Hartje-Zugang und Datenformat:** unbekannt (CSV- oder BMEcat-Feed, veloconnect, API?). Keine Annahmen treffen; bis zur Klärung gilt die Dateischnittstelle mit Testdaten. Am meisten hilft eine echte Beispieldatei.
- **Varianten bei Hartje:** flach oder Eltern-Kind? Sind Modellname, Farbe und Rahmenform sauber genug für die Gruppierung?
- **Development-Store und API-Token:** fehlen. Das Token kommt nur als Umgebungsvariable, nie in den Chat oder ins Repository.
- **Lieferzeit im Theme:** Namespace und Key des Metafelds.
- **Kalkulation:** Faktor, Rundung und UVP als Obergrenze sind nicht festgelegt.
- **Preisregel ([18](decisions/0018-price-changes-need-approval.md)):** Was passiert bei EK-Erhöhungen? Wird der Betrag eins zu eins vom Ladenpreis abgezogen (Annahme bisher) oder brutto umgerechnet?
- **Schutzregeln ([13](decisions/0013-guard-rules.md)):** vom Betreiber nicht ausdrücklich bestätigt.
- **Vorläufige Entscheidungen ohne Betreiber:** handgepflegte API-Typen ([19](decisions/0019-handwritten-api-types.md)), kein Login ([20](decisions/0020-no-login-localhost.md)).

## Bekannte Schwächen
- **Viele Tasks beim Erstimport:** Neue Artikel kommen je Modell als ein Task ([22](decisions/0022-new-model-one-task.md)), ein ganzer Katalog ergibt trotzdem hunderte. Die Taskliste zeigt höchstens 200 Einträge; Filter und Sammelfreigabe fehlen.
- **Modellerkennung:** exakt gleiche Marke, Bezeichnung und Modelljahr. Steckt die Farbe in der Bezeichnung, zerfällt ein Modell.
- **Doppelte Sammel-Tasks:** Jeder Import mit Datenproblemen legt einen neuen Task an, auch bei denselben Problemen wie im Lauf davor.
- **Freigaben ohne Wirkung:** Der eigene Katalog fehlt, eine Freigabe wird nur protokolliert.
- **Angehaltener Import:** Es gibt noch keinen Weg, einen angehaltenen Lauf zu bestätigen.
- **Englische Texte:** Meldungen zu Datenproblemen und Regelnamen sind englisch; die Oberfläche übersetzt nur die Entscheidungsgründe.
- **Gruppierung vergleicht exakt:** „Schwarz“ und „schwarz“ sind zwei Werte.
- **Zeichensatz:** Der CSV-Konnektor erwartet UTF-8; Windows-1252 wird nicht unterstützt.
- **Lieferzeit-Texte:** stehen fest im Code (`availability.Derive`), sollen konfigurierbar werden.
- **Shopify-Grenzen:** 3 Optionen und 2048 Varianten je Produkt stehen als Konstanten im Code und sind gegen die verwendete API-Version zu prüfen.
- **Testdaten:** `testdata/example_supplier.csv` ist erfunden und kein Hartje-Format.
