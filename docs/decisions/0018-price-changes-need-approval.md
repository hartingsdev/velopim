# 18. Preisänderungen immer zur Freigabe, Senkungen betragsgleich

**Status:** Angenommen (Vorgabe des Betreibers), noch nicht umgesetzt

## Kontext
Vorgabe des Betreibers: Preise dürfen nie automatisch geändert werden. Senkt Hartje den Preis um 5 €, soll der eigene Preis auch um 5 € sinken. Auf Nachfrage: gemeint ist der Einkaufspreis; vorerst nur als Vorschlag, später sollen Vorschlag und automatische Übernahme möglich sein.

## Entscheidung
Für `price_change` werden keine `auto`-Regeln angelegt, jede Preisänderung wird ein Task. Senkt der Lieferant den Einkaufspreis um X €, enthält der Vorschlag den eigenen Verkaufspreis minus X €. Der Verkaufspreis wird also nicht bei jeder Änderung neu per Aufschlag kalkuliert: Die Kalkulation setzt ihn beim Anlegen, danach folgt er den Senkungen.

## Alternativen
- **Neukalkulation per Aufschlag:** aus 5 € weniger EK würden bei Faktor 1,6 rund 9,50 € weniger Verkaufspreis; das will der Betreiber nicht.
- **Automatische Übernahme:** vorerst ausgeschlossen, soll später je Regel zuschaltbar sein.

## Folgen
Der EK ist netto, der Verkaufspreis brutto: 5 € weniger Ladenpreis geben netto nur 4,20 € weiter. Vorerst wird der Betrag eins zu eins vom Ladenpreis abgezogen. Offen: Verhalten bei EK-Erhöhungen. Umsetzung erst mit dem eigenen Katalog, vorher gibt es keinen Verkaufspreis.
