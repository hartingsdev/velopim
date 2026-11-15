# 31. Wiedervorlage für Aufgaben und tägliche Mail

**Status:** Angenommen (Vorgabe des Betreibers), umgesetzt; Einzelheiten vorläufig

## Kontext
Manche Aufgaben lassen sich nicht sofort entscheiden. Sie sollen aus der Liste verschwinden und zu einem Termin wiederkommen. Außerdem soll täglich eine Mail daran erinnern, dass Aufgaben offen sind.

## Entscheidung
**Wiedervorlage**
- Eine offene Aufgabe lässt sich bis zu einem Tag zurückstellen (`task.snoozed_until`). Sie bleibt offen, steht aber nicht in der Liste „Offen“, sondern unter „Wiedervorlage“.
- Am Beginn des gewählten Tages ist sie wieder in „Offen“, mit dem Hinweis „Wiedervorlage fällig“.
- Eine zurückgestellte Aufgabe lässt sich jederzeit entscheiden oder von Hand wieder vorlegen.
- Die Zahl am Reiter „Aufgaben“ zählt zurückgestellte Aufgaben nicht mit.

**Tägliche Mail**
- Ein Job sendet einmal am Tag (Standard 07:00, Ortszeit) eine Mail mit der Zahl der offenen Aufgaben, davon mit hoher Priorität und aus der Wiedervorlage zurück, aufgeteilt nach Kategorie, und dem Link zur Aufgabenliste.
- An einem Tag ohne offene Aufgaben wird keine Mail gesendet.
- Einstellungen nur über Umgebungsvariablen (`VELOPIM_SMTP_*`, `VELOPIM_MAIL_*`, `VELOPIM_BASE_URL`). Ohne `VELOPIM_SMTP_HOST` gibt es keine Mail.
- Port 465 nutzt TLS von Beginn an, sonst STARTTLS, wenn der Server es anbietet. Ein Passwort geht nie über eine unverschlüsselte Verbindung zu einem fremden Server.
- `velopim digest` sendet die Mail sofort, zum Ausprobieren der Einstellungen.

## Alternativen
- **Wiedervorlage als eigener Status:** Die Aufgabe wäre dann weder offen noch erledigt; alle Stellen, die offene Aufgaben ersetzen oder schließen, müssten den dritten Status kennen. Ein Datum an der offenen Aufgabe ist einfacher.
- **Mail je Aufgabe:** bei vielen Aufgaben zu viele Mails.

## Folgen
- Ersetzt ein neuerer Import eine zurückgestellte Aufgabe ([16](0016-supersede-pending-actions.md)), ist die neue Aufgabe nicht zurückgestellt.
- Die Mail wird nur gesendet, wenn der Server zur eingestellten Zeit läuft; ein verpasster Termin wird nicht nachgeholt.
- Schlägt der Versand fehl, steht das nur im Protokoll des Servers. Es entsteht keine Aufgabe.
- Mit Gruppen (OIDC) sollte die Mail später je Gruppe nur deren Aufgaben nennen.
