import { useState } from "react";
import { runSync, type PlanProduct, type SyncOp, type SyncPlan, type SyncResult } from "./api";
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

interface Props {
  plan: SyncPlan;
  onChanged: () => Promise<void>;
  onError: (message: string | null) => void;
}

/** What a sync would send to Shopify, and the button that sends it. */
export function Sync({ plan, onChanged, onError }: Props) {
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<SyncResult | null>(null);
  const pending = plan.products.filter((p) => p.ops.length > 0).length;
  const canSend = plan.store_configured && plan.write_enabled;

  async function send() {
    setBusy(true);
    try {
      setResult(await runSync());
      onError(null);
    } catch (e) {
      onError((e as Error).message);
    }
    await onChanged();
    setBusy(false);
  }

  return (
    <>
      <p className={canSend ? "hint" : "error"} role="note">
        {!plan.store_configured &&
          "Es ist kein Shopify-Store eingerichtet. Hier steht nur, was gesendet würde."}
        {plan.store_configured &&
          !plan.write_enabled &&
          "Vorschau (Dry-Run): Der Store ist verbunden, das Schreiben ist ausgeschaltet (SHOPIFY_WRITE_ENABLED=false in der .env). Hier steht, was gesendet würde."}
        {canSend && "Der Store ist verbunden und das Schreiben ist eingeschaltet. Geschrieben wird nur in einen Development-Store."}
      </p>
      <div className="toolbar">
        <button type="button" className="primary" disabled={busy || !canSend || pending === 0} onClick={() => void send()}>
          {busy ? "Überträgt …" : pending === 0 ? "Nichts zu übertragen" : `${pending === 1 ? "1 Produkt" : `${pending} Produkte`} übertragen`}
        </button>
        <span className="hint">
          {plan.products.length === 0
            ? "Kein Produkt ist für den Store ausgewählt."
            : `${plan.products.length === 1 ? "1 Produkt" : `${plan.products.length} Produkte`} im Store oder dafür ausgewählt.`}
        </span>
      </div>
      {result && (
        <div className="task">
          <strong>
            {result.synced} übertragen, {result.unchanged} unverändert, {result.failed} fehlgeschlagen
          </strong>
          <ul className="issues">
            {result.products
              .filter((p) => p.status !== "unchanged")
              .map((p) => (
                <li key={p.product_id}>
                  {p.name}: {p.status === "synced" ? (p.created ? "neu in Shopify angelegt (Entwurf)" : "aktualisiert") : `fehlgeschlagen – ${p.error}`}
                </li>
              ))}
          </ul>
          {result.products.some((p) => p.created) && (
            <p className="hint">
              Für neue Produkte gibt es eine Aufgabe <a href={href("tasks", { category: "shopify" })}>„Veröffentlichen“</a>.
            </p>
          )}
        </div>
      )}
      {plan.products.length === 0 && (
        <p className="empty">
          Wähle ein Produkt in der <a href={href("products")}>Produktansicht</a> mit „In den Store“ aus.
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
    <li className={p.issues.length > 0 || p.last_error ? "task prio-1" : "task"}>
      <div className="task-head">
        <h2>
          <a href={href("product", { id: String(p.product_id) })}>{p.name}</a>
        </h2>
        <span className="meta">
          {!p.wanted && <span className="badge">aus dem Store genommen</span>}
          <span className="badge status-done">{p.in_shopify ? "in Shopify" : "noch nicht in Shopify"}</span>
          {p.last_synced_at && `zuletzt ${formatDate(p.last_synced_at)}`}
        </span>
      </div>
      {p.last_error && <p className="error">Letzter Fehler: {p.last_error}</p>}
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
        <summary>Was Shopify bekommt</summary>
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
