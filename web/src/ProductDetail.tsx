import { useState, type FormEvent } from "react";
import {
  addProductNote,
  ApiError,
  setVariantPrice,
  updateProduct,
  type FieldChange,
  type ProductDetail as Detail,
} from "./api";
import {
  availabilityLabel,
  axisLabel,
  fieldLabel,
  formatAmount,
  formatCents,
  formatDate,
  formatPrice,
  parseAmount,
  productStatusLabel,
} from "./format";
import { href } from "./route";

interface Props {
  detail: Detail;
  onChanged: () => Promise<void>;
  onError: (message: string | null) => void;
}

type ShopProduct = Detail["products"][number];

/** Everything about one model: its shop products, variants, history and internal notes. */
export function ProductDetail({ detail, onChanged, onError }: Props) {
  const first = detail.products[0];
  const splitAxes = first.split.map((s) => s.axis);
  return (
    <>
      <p className="hint">
        <a href={href("products")}>Produkte</a> ›
      </p>
      <h2 className="page-title">
        {first.brand} {first.model_name || first.name}
      </h2>
      <p className="item">
        {[first.model_year, first.category].filter(Boolean).join(" · ")}
        {(first.model_year || first.category) && " · "}
        {detail.products.length === 1
          ? "ein Produkt im Shop"
          : `im Shop ${detail.products.length} Produkte, getrennt nach ${splitAxes.map(axisLabel).join(", ")}`}
      </p>
      {detail.products.map((p) => (
        <ShopProductCard key={p.id} product={p} onChanged={onChanged} onError={onError} />
      ))}
      <Notes detail={detail} onChanged={onChanged} onError={onError} />
      <History detail={detail} />
    </>
  );
}

function ShopProductCard({ product, onChanged, onError }: { product: ShopProduct } & Omit<Props, "detail">) {
  const [name, setName] = useState(product.name);
  const [description, setDescription] = useState(product.description_html);
  const [prices, setPrices] = useState<Record<number, string>>({});
  const [busy, setBusy] = useState(false);
  const [problems, setProblems] = useState<string[]>([]);

  const priceChanges = product.variants.flatMap((v) => {
    const typed = prices[v.id];
    if (typed === undefined || typed.trim() === "") return [];
    const cents = parseAmount(typed);
    return cents === v.price_cents ? [] : [{ v, typed, cents }];
  });
  const textChanged = name.trim() !== product.name || description !== product.description_html;
  const dirty = textChanged || priceChanges.length > 0;

  async function run(action: () => Promise<void>) {
    setBusy(true);
    setProblems([]);
    try {
      await action();
      onError(null);
      setPrices({});
      await onChanged();
    } catch (e) {
      setProblems(e instanceof ApiError && e.issues.length > 0 ? e.issues : [(e as Error).message]);
    }
    setBusy(false);
  }

  function save(e: FormEvent) {
    e.preventDefault();
    const invalid = priceChanges.filter((c) => c.cents === null || c.cents <= 0).map((c) => `${c.v.sku}: „${c.typed}“ ist kein Preis`);
    if (name.trim() === "") invalid.push("Der Name darf nicht leer sein");
    if (invalid.length > 0) {
      setProblems(invalid);
      return;
    }
    void run(async () => {
      if (textChanged) await updateProduct(product.id, { name: name.trim(), description_html: description });
      for (const c of priceChanges) await setVariantPrice(c.v.id, c.cents!);
    });
  }

  const unpriced = product.variants.filter((v) => v.price_cents === null).length;

  return (
    <form className="task" onSubmit={save}>
      <div className="task-head">
        <h2>{product.name}</h2>
        <span className="meta">
          <span className="badge status-done">{productStatusLabel(product.status)}</span>
          {product.wanted && <span className="badge">Für den Store</span>}
        </span>
      </div>

      <div className="rule-form">
        <div className="fields">
          <label className="wide">
            Name im Shop
            <input type="text" value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <label className="wide">
            Beschreibung (HTML)
            <textarea rows={4} value={description} onChange={(e) => setDescription(e.target.value)} />
          </label>
        </div>
      </div>

      {product.image_urls.length > 0 && (
        <p className="item">
          Bilder:{" "}
          {product.image_urls.map((url, i) => (
            <span key={url}>
              {i > 0 && " · "}
              <a href={url} target="_blank" rel="noreferrer noopener">
                {url.split("/").pop() || url}
              </a>
            </span>
          ))}
        </p>
      )}

      <div className="variants scroll">
        <table>
          <thead>
            <tr>
              <th>Artikelnr.</th>
              {product.option_names.map((n) => (
                <th key={n}>{n}</th>
              ))}
              <th>EAN</th>
              <th>Lieferant</th>
              <th>Verfügbarkeit</th>
              <th className="num">EK</th>
              <th className="num">UVP</th>
              <th className="num">Streichpreis</th>
              <th className="num">Verkaufspreis</th>
            </tr>
          </thead>
          <tbody>
            {product.variants.map((v) => (
              <tr key={v.id}>
                <td>{v.sku}</td>
                {product.option_names.map((n, i) => (
                  <td key={n}>{v.option_values[i] || "–"}</td>
                ))}
                <td>{v.ean || "–"}</td>
                <td>{v.supplier_sku ? `${v.source_id} ${v.supplier_sku}` : "–"}</td>
                <td>{availabilityLabel(v.availability)}</td>
                <td className="num">{formatPrice(v.cost_cents)}</td>
                <td className="num">{formatPrice(v.rrp_cents)}</td>
                <td className="num">{formatPrice(v.compare_at_cents)}</td>
                <td className="num">
                  <span className="amount">
                    <input
                      type="text"
                      inputMode="decimal"
                      value={prices[v.id] ?? formatAmount(v.price_cents)}
                      onChange={(e) => setPrices({ ...prices, [v.id]: e.target.value })}
                      aria-label={`Verkaufspreis für ${v.sku} in Euro`}
                    />{" "}
                    €
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {problems.length > 0 && (
        <div className="error" role="alert">
          <ul>
            {problems.map((p) => (
              <li key={p}>{p}</li>
            ))}
          </ul>
        </div>
      )}

      <div className="decide">
        <button type="submit" className="primary" disabled={busy || !dirty}>
          Änderungen speichern
        </button>
        <button
          type="button"
          disabled={busy || dirty || (!product.wanted && unpriced > 0)}
          onClick={() => void run(() => updateProduct(product.id, { wanted: !product.wanted }))}
        >
          {product.wanted ? "Aus dem Store nehmen" : "In den Store"}
        </button>
      </div>
      {!product.wanted && unpriced > 0 && (
        <p className="hint">
          {unpriced === 1 ? "Eine Variante hat" : `${unpriced} Varianten haben`} keinen Verkaufspreis. Erst mit Preisen
          kann das Produkt in den Store.
        </p>
      )}
      {product.wanted && (
        <p className="hint">
          Für den Store ausgewählt. Was gesendet würde, steht unter <a href={href("sync")}>Shopify</a>; übertragen wird
          noch nichts.
        </p>
      )}
    </form>
  );
}

function Notes({ detail, onChanged, onError }: Props) {
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const first = detail.products[0];

  async function add(e: FormEvent) {
    e.preventDefault();
    if (body.trim() === "") return;
    setBusy(true);
    try {
      await addProductNote(first.id, body.trim());
      setBody("");
      onError(null);
      await onChanged();
    } catch (err) {
      onError((err as Error).message);
    }
    setBusy(false);
  }

  return (
    <section className="task">
      <div className="task-head">
        <h2>Interne Notizen</h2>
        <span className="meta">gehen nie zu Shopify</span>
      </div>
      {detail.notes.length === 0 && <p className="item">Noch keine Notiz.</p>}
      <ul className="results">
        {detail.notes.map((n) => (
          <li key={n.id}>
            <span className="note-body">{n.body}</span>
            <span className="meta">
              {n.author} · {formatDate(n.created_at)}
            </span>
          </li>
        ))}
      </ul>
      <form className="decide" onSubmit={(e) => void add(e)}>
        <input
          type="text"
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="Notiz, z. B. Ansprechpartner oder Bestellhinweis"
          aria-label="Neue interne Notiz"
          maxLength={2000}
        />
        <button type="submit" disabled={busy || body.trim() === ""}>
          Notiz speichern
        </button>
      </form>
    </section>
  );
}

const historyFields: Record<string, string> = {
  price_cents: "Verkaufspreis",
  compare_at_cents: "Streichpreis",
  description_html: "Beschreibung",
  image_urls: "Bilder",
  name: "Name",
  sku: "Artikelnummer",
  status: "Status",
  in_store: "Für den Store",
  ean: "EAN",
};

function historyValue(field: string, value: string | null): string {
  if (value === null || value === "") return "–";
  if (field.endsWith("_cents")) return formatCents(Number(value));
  if (field === "in_store") return value === "true" ? "ja" : "nein";
  if (field === "status") return productStatusLabel(value);
  if (field === "image_urls") {
    try {
      const urls = JSON.parse(value) as string[];
      return urls.length === 1 ? "1 Bild" : `${urls.length} Bilder`;
    } catch {
      return value;
    }
  }
  return value.length > 80 ? `${value.slice(0, 80)}…` : value;
}

function origin(c: FieldChange): string {
  switch (c.origin) {
    case "user":
      return c.source ? `${c.actor ?? "Person"} (Freigabe, ${c.source})` : (c.actor ?? "Person");
    case "rule":
      return `Regel${c.rule ? ` „${c.rule}“` : ""}${c.source ? `, ${c.source}` : ""}`;
    case "import":
      return `Import${c.source ? ` ${c.source}` : ""}`;
    default:
      return "System";
  }
}

function History({ detail }: { detail: Detail }) {
  const names = new Map(detail.products.map((p) => [p.id, p.name]));
  return (
    <section className="task">
      <div className="task-head">
        <h2>Herkunft und Verlauf</h2>
        <span className="meta">{detail.history.length === 300 ? "die letzten 300 Änderungen" : `${detail.history.length} Änderungen`}</span>
      </div>
      <div className="variants scroll">
        <table>
          <thead>
            <tr>
              <th>Wann</th>
              <th>Was</th>
              <th>Feld</th>
              <th>Vorher</th>
              <th>Nachher</th>
              <th>Woher</th>
              <th>Grund</th>
            </tr>
          </thead>
          <tbody>
            {detail.history.map((c) => (
              <tr key={c.id}>
                <td>{formatDate(c.created_at)}</td>
                <td>{c.variant_sku ? `Variante ${c.variant_sku}` : (names.get(c.product_id) ?? "Produkt")}</td>
                <td>{historyFields[c.field] ?? fieldLabel(c.field)}</td>
                <td>{historyValue(c.field, c.old)}</td>
                <td>{historyValue(c.field, c.new)}</td>
                <td>{origin(c)}</td>
                <td>{c.reason ?? "–"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
