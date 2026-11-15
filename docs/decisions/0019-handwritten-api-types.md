# 19. API-Typen von Hand gepflegt

**Status:** Vorläufig, ohne Betreiber entschieden

## Kontext
Go-Backend und React-Oberfläche brauchen dieselben Datentypen.

## Entscheidung
Die TypeScript-Typen in `web/src/api.ts` werden von Hand passend zu `internal/api/api.go` gepflegt.

## Alternativen
- **OpenAPI mit Codegenerierung:** hält beide Seiten sicher synchron, braucht aber eine Spezifikation und zwei Generatoren.

## Folgen
Bei vier Endpunkten lohnt die Generierung nicht. Wächst die API deutlich, neu bewerten. Risiko bis dahin: Beide Seiten laufen auseinander, ohne dass der Compiler es merkt.
