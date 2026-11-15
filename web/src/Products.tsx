import type { Product, ProductList } from "./api";
import { availabilityLabel, axisLabel, formatPrice, productStatusLabel } from "./format";
import { Chip, href, SearchBox, type Params } from "./route";

interface Model {
  key: string;
  brand: string;
  name: string;
  products: Product[];
}

/** Products cut from one model are shown together: the cut is for customers, not for us. */
function groupByModel(products: Product[]): Model[] {
  const models = new Map<string, Model>();
  for (const p of products) {
    const key = p.model_key || `product-${p.id}`;
    let model = models.get(key);
    if (!model) {
      model = { key, brand: p.brand, name: p.model_name || p.name, products: [] };
      models.set(key, model);
    }
    model.products.push(p);
  }
  return [...models.values()];
}

export function Products({ list, params }: { list: ProductList; params: Params }) {
  const filtered = Boolean(params.q || params.status || params.unpriced);
  const models = groupByModel(list.products);
  return (
    <>
      <div className="toolbar">
        <div className="chips" role="group" aria-label="Filter">
          <Chip view="products" params={{ q: params.q }} active={!params.status && !params.unpriced}>
            Alle <span className="n">{list.total}</span>
          </Chip>
          <Chip view="products" params={{ q: params.q, status: "draft" }} active={params.status === "draft"}>
            Entwurf
          </Chip>
          <Chip view="products" params={{ q: params.q, status: "active" }} active={params.status === "active"}>
            Aktiv
          </Chip>
          <Chip view="products" params={{ q: params.q, unpriced: "true" }} active={params.unpriced === "true"}>
            Ohne Preis <span className="n">{list.unpriced}</span>
          </Chip>
        </div>
        <SearchBox view="products" params={params} placeholder="Produkte durchsuchen (Name, Marke, Artikelnr., EAN)" />
      </div>
      {models.length === 0 && (
        <p className="empty">
          {filtered
            ? "Kein Produkt passt zu Suche und Filter."
            : "Noch keine Produkte. Sie entstehen, wenn du ein neues Modell freigibst."}
        </p>
      )}
      <ul className="tasks">
        {models.map((model) => (
          <ModelCard key={model.key} model={model} />
        ))}
      </ul>
    </>
  );
}

function ModelCard({ model }: { model: Model }) {
  const first = model.products[0];
  const splitAxes = first.split.map((s) => s.axis);
  // Columns: what separates the shop products, then their options.
  const optionNames = [...new Set(model.products.flatMap((p) => p.option_names))];
  const statuses = [...new Set(model.products.map((p) => p.status))];
  const rows = model.products.flatMap((p) => p.variants.map((v) => ({ p, v })));
  return (
    <li className="task">
      <div className="task-head">
        <h2>
          <a href={href("product", { id: String(first.id) })}>
            {model.brand} {model.name}
          </a>
        </h2>
        <span className="meta">
          {statuses.map((s) => (
            <span key={s} className="badge status-done">
              {productStatusLabel(s)}
            </span>
          ))}
        </span>
      </div>
      <p className="item">
        {[first.model_year, first.category].filter(Boolean).join(" · ")}
        {(first.model_year || first.category) && " · "}
        {model.products.length === 1
          ? "ein Produkt im Shop"
          : `im Shop ${model.products.length} Produkte, getrennt nach ${splitAxes.map(axisLabel).join(", ")}`}
      </p>
      <details className="variants" open={rows.length <= 12}>
        <summary>{rows.length === 1 ? "1 Variante" : `${rows.length} Varianten`}</summary>
        <div className="scroll">
          <table>
            <thead>
              <tr>
                <th>Artikelnr.</th>
                {splitAxes.map((axis) => (
                  <th key={axis}>{axisLabel(axis)}</th>
                ))}
                {optionNames.map((name) => (
                  <th key={name}>{name}</th>
                ))}
                <th>Lieferant</th>
                <th>Verfügbarkeit</th>
                <th className="num">EK</th>
                <th className="num">UVP</th>
                <th className="num">Verkaufspreis</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(({ p, v }) => (
                <tr key={v.id}>
                  <td>{v.sku}</td>
                  {p.split.map((s) => (
                    <td key={s.axis}>{s.value || "–"}</td>
                  ))}
                  {optionNames.map((name) => (
                    <td key={name}>{v.option_values[p.option_names.indexOf(name)] || "–"}</td>
                  ))}
                  <td>{v.supplier_sku ? `${v.source_id} ${v.supplier_sku}` : "–"}</td>
                  <td>{availabilityLabel(v.availability)}</td>
                  <td className="num">{formatPrice(v.cost_cents)}</td>
                  <td className="num">{formatPrice(v.rrp_cents)}</td>
                  <td className="num">{formatPrice(v.price_cents)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </details>
    </li>
  );
}
