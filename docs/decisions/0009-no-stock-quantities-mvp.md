# 9. Bestand im MVP: nur „verkaufbar“ und Lieferzeit

**Status:** Angenommen

## Kontext
Der Betreiber hat etwas auf Lager, führt den Bestand aber nicht. Lieferantenbestände sind kein eigener Bestand.

## Entscheidung
velopim schreibt keine Stückzahlen zu Shopify. Es setzt nur, ob eine Variante verkaufbar ist, und ein Metafeld mit dem Lieferzeit-Text.

## Alternativen
- **Lieferantenbestand als Shopify-Bestand:** Shopify würde Mengen anzeigen und herunterzählen, die dem Shop nicht gehören.

## Folgen
Später kann Eigenbestand über eine eigene Shopify-Location dazukommen. Namespace und Key des Metafelds sind noch offen.

Nachtrag: Der Betreiber hat bestätigt, dass ein eigener Bestand nicht gepflegt würde. Eine Bestandsführung in Shopify oder velopim wird deshalb nicht gebaut.
