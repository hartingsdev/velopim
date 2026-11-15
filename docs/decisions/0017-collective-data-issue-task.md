# 17. Datenprobleme eines Imports ergeben einen Sammel-Task

**Status:** Angenommen

## Kontext
Ein Import kann tausende Datenprobleme haben (fehlende EAN, unplausibles Modelljahr, unlesbarer Preis).

## Entscheidung
Je Importlauf entsteht ein einziger Task. Die einzelnen Probleme stehen in `import_run.summary` (höchstens 500 gespeichert) und sind in der Importansicht aufklappbar.

## Alternativen
- **Ein Task je Problem:** flutet die Taskliste.

## Folgen
Bekannte Schwäche: Jeder Lauf mit Problemen legt einen neuen Sammel-Task an, auch wenn es dieselben Probleme wie im Lauf davor sind.
