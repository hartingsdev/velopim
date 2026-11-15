# Development-Store für Shopify einrichten

velopim schreibt nie in den Live-Shop. Zum Testen braucht es einen eigenen Development-Store und eine App, über die velopim auf die Admin-API zugreift. Stand der Shopify-Doku: November 2026 ([Dev stores](https://shopify.dev/docs/apps/build/stores/development-stores), [Client credentials](https://shopify.dev/docs/apps/build/authentication-authorization/client-credentials-grant)).

## 1. Store anlegen
1. Im Dev Dashboard von Shopify anmelden (Link auf der Doku-Seite „Dev stores“) (Partner-Konto oder Händlerkonto mit Entwicklerrechten).
2. Links **Stores** wählen, dann **Create store** und als Typ **Dev**.
3. Namen vergeben. Er bestimmt die Adresse `name.myshopify.com` und lässt sich später nicht ändern.
4. Als Plan **Basic** wählen, wie im Live-Shop, damit dieselben Grenzen gelten.
5. **Create store**, danach im Store anmelden.

## 2. App anlegen und installieren
1. Im Dev Dashboard unter **Apps** eine App anlegen.
2. In der App-Version die Zugriffsrechte (Access scopes) wählen: `read_products`, `write_products`, `read_inventory`, `write_inventory`, `read_locations`, `write_publications` (für die Veröffentlichung im Kanal „Onlineshop“).
3. Die App im Development-Store installieren.

## 3. Zugangsdaten eintragen
In den Einstellungen der App (**Settings**) stehen **Client ID** und **Client secret**. Beide kommen in die Datei `.env` im Projektordner, nicht in den Chat und nicht ins Repository:

```
SHOPIFY_STORE_DOMAIN=name.myshopify.com
SHOPIFY_CLIENT_ID=…
SHOPIFY_CLIENT_SECRET=…
SHOPIFY_WRITE_ENABLED=false
```

Im Formular „Create app manually“ genügt: Name, App URL `https://example.com`, „Embed app in Shopify admin“ aus, die Scopes oben. Installiert wird über **Test app** im Kasten „Installs“.

Prüfen, ob die Verbindung steht (liest nur): `./bin/velopim shopify-check`

## Rechte später ergänzen
Im Dev Dashboard die App öffnen, links **Versions**, dann **New version**. Im Feld **Scopes** das neue Recht anhängen, zum Beispiel `…,read_locations,write_publications`, und die Version veröffentlichen. Danach die App im Store öffnen (Einstellungen → Apps) und die neuen Rechte bestätigen. `./bin/velopim shopify-check` zeigt, welche Rechte das Token hat.

Ein festes Admin-Token gibt es bei diesem Weg nicht. velopim tauscht Client ID und Secret gegen ein Token, das 24 Stunden gilt, und erneuert es selbst. `SHOPIFY_WRITE_ENABLED` bleibt `false`, bis der Dry-Run geprüft ist.
