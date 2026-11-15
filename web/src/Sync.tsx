import type { PlanProduct, SyncOp, SyncPlan } from "./api";
import { formatDate, formatPrice } from "./format";
import { href } from "./route";

const fieldNames: Record<string, string> = {
  title: "Name",
  description_html: "Beschreibung",
  vendor: "Marke",
  product_type: "Produkttyp",
  published: "veröffentlicht",
  options: "Optionen",
  barcode: "EAN",
  option_values: "Optionswerte",
  price: "Preis",
  compare_at_price: "Streichpreis",
  sellable: "verkaufbar",
  delivery_text: "Lieferzeit-Text",
};

function opText(op: SyncOp): string {
  const fields = (op.fields ?? []).map((f) => fieldNames[f] ?? f).join(", ");
  switch (op.kind) {
    case "create_product":
      return "Produkt neu anlegen, als Entwurf, mit allen Varianten und Bildern";
    case "update_product":
      return `Produkt ändern: ${fields}`;
    case "set_images":
      return "Bilder ersetzen";
    case "set_metafields":
      return `Metafelder setzen: ${fields}`;
    case "create_variant":
      return `Variante ${op.sku} anlegen`;
    case "update_variant":
      return `Variante ${op.sku} ändern: ${fields}`;
    case "delete_variant":
      return `Variante ${op.sku} löschen`;
  }
}

/** What a sync would send to Shopify. Nothing is sent from here. */
export function Sync({ plan }: { plan: SyncPlan }) {
  const pending = plan.products.filter((p) => p.ops.length > 0).length;
  return (
    <>
      <p className="error" role="note">
        Vorschau (Dry-Run): Hier steht, was an Shopify gesendet würde. Gesendet wird nichts, die Übertragung ist noch
        nicht gebaut.{" "}
        {plan.store_configured
          ? "Zugangsdaten für einen Store sind eingetragen."
          : "Es sind noch keine Zugangsdaten für einen Development-Store eingetragen."}
      </p>
      {plan.products.length === 0 ? (
        <p className="empty">
          Kein Produkt ist für den Store ausgewählt. Wähle es in der <a href={href("products")}>Produktansicht</a> mit „In
          den Store“ aus.
        </p>
      ) : (
        <p className="hint">
          {plan.products.length === 1 ? "1 Produkt ist" : `${plan.products.length} Produkte sind`} für den Store
          ausgewählt, {pending === 0 ? "keines hat" : pending === 1 ? "eines hat" : `${pending} haben`} offene Änderungen.
        </p>
      )}
      <ul className="tasks">
        {plan.products.map((p) => (
          <PlanCard key={p.product_id} product={p} />
        ))}
      </ul>
    </>
  );
}

function PlanCard({ product: p }: { product: PlanProduct }) {
  return (
    <li className={p.issues.length > 0 ? "task prio-1" : "task"}>
      <div className="task-head">
        <h2>
          <a href={href("product", { id: String(p.product_id) })}>{p.name}</a>
        </h2>
        <span className="meta">
          <span className="badge status-done">{p.in_shopify ? "in Shopify" : "noch nicht in Shopify"}</span>
          {p.last_synced_at && `zuletzt ${formatDate(p.last_synced_at)}`}
        </span>
      </div>
      {p.last_error && <p className="error">{p.last_error}</p>}
      {p.issues.length > 0 && (
        <div className="error">
          Shopify würde das Produkt so ablehnen:
          <ul>
            {p.issues.map((issue) => (
              <li key={issue}>{issue}</li>
            ))}
          </ul>
        </div>
      )}
      {p.ops.length === 0 ? (
        <p className="item">Keine Änderungen zu übertragen.</p>
      ) : (
        <ul className="issues">
          {p.ops.map((op, i) => (
            <li key={i}>{opText(op)}</li>
          ))}
        </ul>
      )}
      <details className="variants">
        <summary>Was Shopify bekäme</summary>
        <p className="item">
          {p.projection.title} · {p.projection.vendor || "ohne Marke"} · {p.projection.product_type || "ohne Produkttyp"} ·{" "}
          {p.projection.published ? "veröffentlicht" : "Entwurf"} · {p.projection.image_urls.length} Bilder ·{" "}
          {p.projection.description_html ? "mit Beschreibung" : "ohne Beschreibung"}
        </p>
        <div className="scroll">
          <table>
            <thead>
              <tr>
                <th>Artikelnr.</th>
                {p.projection.options.map((o) => (
                  <th key={o}>{o}</th>
                ))}
                <th>EAN</th>
                <th className="num">Preis</th>
                <th className="num">Streichpreis</th>
                <th>Verkaufbar</th>
                <th>Lieferzeit-Text</th>
              </tr>
            </thead>
            <tbody>
              {p.projection.variants.map((v) => (
                <tr key={v.sku}>
                  <td>{v.sku}</td>
                  {p.projection.options.map((o, i) => (
                    <td key={o}>{v.option_values[i] || "–"}</td>
                  ))}
                  <td>{v.barcode || "–"}</td>
                  <td className="num">{formatPrice(v.price_cents || null)}</td>
                  <td className="num">{formatPrice(v.compare_at_cents || null)}</td>
                  <td>{v.sellable ? "ja" : "nein"}</td>
                  <td>{v.delivery_text}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </details>
    </li>
  );
}
