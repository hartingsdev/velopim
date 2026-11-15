# 1. Go als Backend-Sprache

**Status:** Angenommen (Betreiber: kein TypeScript/Node im Backend, „eher Richtung Go, Rust“)

## Kontext
Der Betreiber will ein wartungsarmes Backend ohne Node. Die Anwendung ist datenlastig (Importe, Vergleiche, Sync), aber nicht rechenkritisch.

## Entscheidung
Go, ein einzelnes Binary, HTTP mit `net/http` aus der Standardbibliothek.

## Alternativen
- **Rust:** mehr Aufwand bei Entwicklung und Einarbeitung, der Gewinn an Geschwindigkeit und Typsicherheit wird hier nicht gebraucht.
- **TypeScript/Node:** vom Betreiber ausgeschlossen.

## Folgen
Schnelle Übersetzung, einfache Auslieferung, wenige Abhängigkeiten. Die Oberfläche braucht trotzdem eine Node-Build-Kette, siehe [0003](0003-react-frontend.md).
