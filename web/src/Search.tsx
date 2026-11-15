import type { ProductList, SupplierItem, TaskList } from "./api";
import { availabilityLabel, categoryLabel, formatDate, formatPrice } from "./format";
import { href } from "./route";

export interface SearchResults {
  q: string;
  open: TaskList;
  closed: TaskList;
  products: ProductList;
  items: SupplierItem[];
}

/** Results of the search across everything, with links into the lists. */
export function Search({ results }: { results: SearchResults }) {
  const { q, open, closed, products, items } = results;
  if (!q) {
    return <p className="empty">Suche oben nach Artikelnummer, EAN, Marke oder Bezeichnung.</p>;
  }
  const tasks = [...open.tasks, ...closed.tasks];
  const nothing = tasks.length === 0 && products.products.length === 0 && items.length === 0;
  return (
    <>
      <p className="hint">Suche nach „{q}“</p>
      {nothing && <p className="empty">Nichts gefunden.</p>}

      {tasks.length > 0 && (
        <section className="task">
          <div className="task-head">
            <h2>Aufgaben</h2>
            <span className="meta">
              {open.tasks.length > 0 && <a href={href("tasks", { q })}>{open.tasks.length} offen</a>}
              {closed.tasks.length > 0 && <a href={href("tasks", { q, status: "closed" })}>{closed.tasks.length} erledigt</a>}
            </span>
          </div>
          <ul className="results">
            {tasks.slice(0, 10).map((t) => (
              <li key={t.id}>
                <a href={href("tasks", { q, status: t.status === "open" ? undefined : "closed" })}>{t.title}</a>
                <span className="meta">
                  {categoryLabel(t.category)} · {t.status === "open" ? "offen" : `erledigt ${formatDate(t.closed_at)}`}
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}

      {products.products.length > 0 && (
        <section className="task">
          <div className="task-head">
            <h2>Produkte</h2>
            <span className="meta">
              <a href={href("products", { q })}>{products.products.length} anzeigen</a>
            </span>
          </div>
          <ul className="results">
            {products.products.slice(0, 10).map((p) => (
              <li key={p.id}>
                <a href={href("product", { id: String(p.id) })}>
                  {p.brand} {p.name}
                </a>
                <span className="meta">
                  {p.variants.length === 1 ? "1 Variante" : `${p.variants.length} Varianten`}
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}

      {items.length > 0 && (
        <section className="task">
          <div className="task-head">
            <h2>Lieferantenartikel</h2>
            <span className="meta">{items.length === 50 ? "die ersten 50" : items.length}</span>
          </div>
          <div className="variants scroll">
            <table>
              <thead>
                <tr>
                  <th>Lieferant</th>
                  <th>Artikelnr.</th>
                  <th>Bezeichnung</th>
                  <th>Farbe</th>
                  <th>Rahmenhöhe</th>
                  <th>Verfügbarkeit</th>
                  <th className="num">EK</th>
                  <th className="num">UVP</th>
                  <th>Eigene Nr.</th>
                </tr>
              </thead>
              <tbody>
                {items.map((it) => (
                  <tr key={it.id} className={it.active ? undefined : "inactive"}>
                    <td>{it.source_id}</td>
                    <td>{it.sku}</td>
                    <td>
                      {it.brand} {it.name} {it.model_year ?? ""}
                    </td>
                    <td>{it.color || "–"}</td>
                    <td>{it.frame_size || "–"}</td>
                    <td>{it.active ? availabilityLabel(it.availability) : "nicht mehr im Feed"}</td>
                    <td className="num">{formatPrice(it.cost_cents)}</td>
                    <td className="num">{formatPrice(it.rrp_cents)}</td>
                    <td>{it.own_sku ? <a href={href("products", { q: it.own_sku })}>{it.own_sku}</a> : "–"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </>
  );
}
