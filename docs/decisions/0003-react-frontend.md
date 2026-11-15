# 3. React-Frontend, eingebettet ins Binary

**Status:** Angenommen (Wunsch des Betreibers)

## Kontext
Die Oberfläche ist ein Arbeitswerkzeug für eine Person. Mein Vorschlag war serverseitiges HTML mit htmx, um Node ganz zu vermeiden. Der Betreiber wollte ausdrücklich React.

## Entscheidung
React + TypeScript + Vite in `web/`. Das Build-Ergebnis (`web/dist`) wird per `go:embed` ins Binary gepackt.

## Alternativen
- **htmx / Go-Templates:** keine Build-Kette, aber vom Betreiber nicht gewünscht.

## Folgen
Zum Bauen wird Node gebraucht, in Produktion nicht. `web/dist/.gitkeep` ist eingecheckt, damit das Go-Projekt auch ohne gebaute Oberfläche übersetzt; der Server zeigt dann einen Hinweis.
