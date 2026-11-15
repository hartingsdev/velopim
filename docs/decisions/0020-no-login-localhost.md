# 20. Kein Login, nur localhost

**Status:** Vorläufig, ohne Betreiber entschieden

## Kontext
Es gibt einen Nutzer, der die Anwendung lokal betreibt.

## Entscheidung
Der Server lauscht standardmäßig auf `127.0.0.1`. Es gibt keine Anmeldung. Schreibende Anfragen müssen `Content-Type: application/json` haben; damit kann eine fremde Website sie nicht über ein Formular im Browser auslösen. Wer entschieden hat, kommt aus `VELOPIM_ACTOR`.

## Alternativen
- **Login von Anfang an:** Aufwand ohne Nutzen, solange nur lokal gearbeitet wird.

## Folgen
Vor jedem Betrieb im Netz braucht es eine Anmeldung. `VELOPIM_ADDR` darf bis dahin nicht auf eine öffentliche Adresse gesetzt werden.
