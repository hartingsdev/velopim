import type { Product } from "./api";
import { availabilityLabel, formatPrice, productStatusLabel } from "./format";

export function Products({ products }: { products: Product[] }) {
  if (products.length === 0) {
    return <p className="empty">Noch keine Produkte. Sie entstehen, wenn du ein neues Modell freigibst.</p>;
  }
  return (
    <ul className="tasks">
      {products.map((p) => (
        <li key={p.id} className="task">
          <div className="task-head">
            <h2>
              {p.brand} {p.name}
            </h2>
            <span className="meta">
              <span className="badge status-done">{productStatusLabel(p.status)}</span>
            </span>
          </div>
          <p className="item">{[p.model_year, p.category].filter(Boolean).join(" · ")}</p>
          <details className="variants" open={p.variants.length <= 12}>
            <summary>{p.variants.length === 1 ? "1 Variante" : `${p.variants.length} Varianten`}</summary>
            <div className="scroll">
              <table>
                <thead>
                  <tr>
                    <th>Artikelnr.</th>
                    {p.option_names.map((name) => (
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
                  {p.variants.map((v) => (
                    <tr key={v.id}>
                      <td>{v.sku}</td>
                      {p.option_names.map((name, i) => (
                        <td key={name}>{v.option_values[i] || "–"}</td>
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
      ))}
    </ul>
  );
}
