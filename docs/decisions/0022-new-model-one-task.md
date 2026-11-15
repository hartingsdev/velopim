# 22. Ein neues Modell ist eine Aktion und ein Task

**Status:** Angenommen (vom Betreiber bestätigt), umgesetzt

## Kontext
Bisher wurde jeder neue Lieferantenartikel eine eigene Aktion mit eigenem Task. Ein Produkt entsteht aber je Modell, und der Erstimport eines ganzen Katalogs hätte tausende Tasks erzeugt.

## Entscheidung
Die in einem Importlauf neuen Artikel werden nach Modell zusammengefasst (Quelle, Marke, Bezeichnung, Modelljahr). Je Modell entsteht eine Aktion `new_product` und, wenn die Regeln Freigabe verlangen, ein Task.

- Die Aktion trägt `model_key`; welche Artikel sie umfasst, steht in `action_item`.
- Kommt ein weiterer Artikel eines Modells, dessen Aktion noch offen ist, ersetzt eine neue Aktion die alte und übernimmt deren Artikel. Je Modell gibt es also höchstens einen offenen Task.
- Ist das Modell schon freigegeben oder automatisch übernommen, heißt der Task „Neue Varianten prüfen“ und umfasst nur die neuen Artikel.
- Ein Artikel ohne Bezeichnung lässt sich nicht gruppieren und ist ein Modell für sich.
- Die Regeln werden je Modell ausgewertet, mit Marke und Kategorie. Haben die Artikel verschiedene Kategorien, greifen nur Regeln ohne Kategorie.

## Alternativen
- **Ein Task je Artikel mit Sammelfreigabe in der Oberfläche:** die Flut bleibt in der Datenbank, und die Entscheidung passt nicht zur Einheit „Produkt“.
- **Regel „Neuprodukte automatisch“:** löst die Flut, nimmt dem Betreiber aber die Prüfung.

## Folgen
- Der Modellschlüssel enthält die Quelle. Dasselbe Modell von einem zweiten Lieferanten ist eine Frage des Matchings ([11](0011-staged-matching.md)), kein gemeinsamer Task.
- Das Modell wird über exakt gleiche Marke, Bezeichnung und Modelljahr erkannt. Steckt bei einem Lieferanten die Farbe in der Bezeichnung, zerfällt ein Modell in viele; das zeigt erst eine echte Datei.
- Die Freigabe legt noch kein Produkt an; das kommt mit dem eigenen Katalog. Dort sollen im selben Task auch die Trennmerkmale ([21](0021-configurable-product-split.md)) gewählt werden.
- Bestehende Aktionen je Artikel aus früheren Läufen bleiben unverändert.
