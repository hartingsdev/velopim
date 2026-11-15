# 17. Datenprobleme eines Imports ergeben einen Sammel-Task

**Status:** Angenommen

## Kontext
Ein Import kann tausende Datenprobleme haben (fehlende EAN, unplausibles Modelljahr, unlesbarer Preis).

## Entscheidung
Je Importlauf entsteht ein einziger Task. Die einzelnen Probleme stehen in `import_run.summary` (höchstens 500 gespeichert) und sind in der Importansicht aufklappbar.

## Alternativen
- **Ein Task je Problem:** flutet die Taskliste.

## Folgen
Je Quelle gibt es höchstens einen offenen Sammel-Task. Bleiben die Probleme von Lauf zu Lauf gleich, entsteht kein neuer, auch nicht, nachdem der Betreiber ihn geschlossen hat. Ändern sie sich, ersetzt ein neuer Task den alten.
