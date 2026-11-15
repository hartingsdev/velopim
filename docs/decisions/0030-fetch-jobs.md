# 30. velopim holt Lieferantendateien selbst ab, über Jobs in River

**Status:** Angenommen (Vorgabe des Betreibers: „velopim soll auch abholen können mit Jobs“); Ausgestaltung vorläufig. Im laufenden Server gegen einen lokalen Testserver geprüft

## Kontext
Bisher kam eine Lieferung nur per Upload oder Befehl ins System. Der Betreiber will, dass velopim Daten auch selbst abholt, zeitgesteuert. Was Hartje anbietet, ist unbekannt.

## Entscheidung
- **Abruf:** Je Lieferant lässt sich eine Adresse (`http`/`https`) hinterlegen, von der velopim die Datei lädt, mit Abstand (ab 5 Minuten oder „nur auf Knopfdruck“) und optionaler Anmeldung: Benutzername und Passwort, Bearer-Token oder ein Schlüssel in einem Kopffeld. Die geladene Datei läuft durch dieselbe Import-Pipeline wie ein Upload.
- **Geheimnisse:** Die Einstellung enthält nie ein Passwort, nur den Namen einer Umgebungsvariable. Der Name muss mit `VELOPIM_FETCH_` beginnen. Sonst könnte eine Einstellung beliebige Geheimnisse des Prozesses (Datenbankpasswort, Shopify-Secret) an einen fremden Server senden.
- **Jobs:** River, Queue in PostgreSQL, wie im Tech-Stack vorgesehen ([2](0002-postgresql.md)). Die Jobs laufen im Serverprozess (`velopim serve`), einer nach dem anderen. River legt seine Tabellen beim Start selbst an.
  - `schedule_fetches` läuft jede Minute und stellt für jeden fälligen Lieferanten einen Abruf ein.
  - `fetch_source` lädt und importiert die Datei eines Lieferanten.
  - `shopify_sync` überträgt zu Shopify, wenn `VELOPIM_SYNC_INTERVAL` gesetzt ist.
- **Keine automatische Wiederholung:** Ein fehlgeschlagener Abruf wird nicht sofort wiederholt, sondern zum nächsten Termin erneut versucht.
- **Fehler:** Der Fehler steht am Lieferanten und wird ein Task (höchstens ein offener je Lieferant); der nächste erfolgreiche Abruf schließt ihn.
- **Nie doppelt:** Für denselben Lieferanten wartet oder läuft höchstens ein Abruf.

## Alternativen
- **Eigener kleiner Zeitplaner ohne Queue:** weniger Abhängigkeiten, aber kein gespeicherter Zustand über Neustarts und keine Sperre gegen doppelte Läufe.
- **Cronjob, der den Upload aufruft:** geht weiterhin, liegt aber außerhalb von velopim und meldet Fehler nicht als Task.

## Folgen
- Abgeholt wird nur, solange der Server läuft.
- River zählt in der Standardeinstellung auch erledigte Jobs als „schon vorhanden“; jeder Abruf wäre dann nur einmal am Tag möglich gewesen. Die Eindeutigkeit gilt deshalb ausdrücklich nur für wartende und laufende Jobs (`onePending` in `internal/jobs/queue.go`). Aufgefallen ist das erst im echten Durchlauf.
- Unterstützt wird bisher nur „eine Datei von einer Adresse“. Eine API mit mehreren Aufrufen, Seitenweise-Abruf oder ein anderes Format (BMEcat, veloconnect) braucht einen eigenen Konnektor; die Jobs und die Fehlerbehandlung bleiben dieselben.
- Der Abstand gilt ab dem letzten Versuch, auch wenn er fehlgeschlagen ist.
