# 13. Schutzregeln erzwingen Freigabe

**Status:** Angenommen, vom Betreiber nicht ausdrücklich bestätigt

## Kontext
„Spezifischste Regel gewinnt“ hat eine Lücke: Eine Markenregel „Preisänderungen automatisch“ würde eine allgemeine Regel „unter Mindestmarge immer Freigabe“ aushebeln.

## Entscheidung
Regeln können als Schutzregel (`guard`) markiert werden. Passt eine Schutzregel, gilt Freigabe, egal wie spezifisch andere Regeln sind. Schutzregeln können nur Freigabe verlangen.

## Alternativen
- **Ohne Schutzregeln:** Sicherheitsgrenzen müssten in jeder spezifischen Regel wiederholt werden.

## Folgen
Bei Preisen sind Schutzregeln derzeit ohne Wirkung, weil Preisänderungen ohnehin immer zur Freigabe gehen ([0018](0018-price-changes-need-approval.md)). Sie werden relevant, sobald automatische Preisübernahme zugeschaltet wird.
