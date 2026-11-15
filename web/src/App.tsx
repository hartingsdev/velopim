import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  listImportRuns,
  listPricingRules,
  listProducts,
  listTasks,
  searchSupplierItems,
  type ImportRun,
  type PricingRule,
  type ProductList,
  type TaskList,
} from "./api";
import { ImportRuns } from "./ImportRuns";
import { Pricing } from "./Pricing";
import { Products } from "./Products";
import { href, useRoute, type Route, type View } from "./route";
import { Search, type SearchResults } from "./Search";
import { Tasks } from "./Tasks";

type Data =
  | { view: "tasks"; tasks: TaskList }
  | { view: "products"; products: ProductList }
  | { view: "imports"; runs: ImportRun[] }
  | { view: "pricing"; rules: PricingRule[] }
  | { view: "search"; results: SearchResults };

async function fetchData({ view, params }: Route): Promise<Data> {
  switch (view) {
    case "products":
      return { view, products: await listProducts({ status: params.status, unpriced: params.unpriced, q: params.q }) };
    case "imports":
      return { view, runs: await listImportRuns() };
    case "pricing":
      return { view, rules: await listPricingRules() };
    case "search": {
      const q = (params.q ?? "").trim();
      if (!q) {
        // Nothing to search for: only fetch the counter for the tab.
        const open = await listTasks({ category: "other" });
        const none = { ...open, tasks: [] };
        return { view, results: { q, open: none, closed: none, products: { products: [], total: 0, unpriced: 0 }, items: [] } };
      }
      const [open, closed, products, items] = await Promise.all([
        listTasks({ q }),
        listTasks({ q, status: "closed" }),
        listProducts({ q }),
        searchSupplierItems(q),
      ]);
      return { view, results: { q, open, closed, products, items } };
    }
    default:
      return { view, tasks: await listTasks({ status: params.status, category: params.category, q: params.q }) };
  }
}

export function App() {
  const route = useRoute();
  // What to fetch. Imports and rules are filtered in the browser, so their
  // search text is not part of it.
  const serverSide = route.view === "imports" || route.view === "pricing" ? { view: route.view, params: {} } : route;
  const key = href(serverSide.view, serverSide.params);

  const [data, setData] = useState<{ key: string; data: Data } | null>(null);
  const [openCount, setOpenCount] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const next = await fetchData(serverSide);
      setData({ key, data: next });
      if (next.view === "tasks") setOpenCount(next.tasks.open);
      if (next.view === "search") setOpenCount(next.results.open.open);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- key stands for serverSide
  }, [key]);

  useEffect(() => {
    void load();
  }, [load]);

  const tab = (target: View, label: string, count?: number | null) => (
    <a href={href(target)} className={route.view === target ? "tab active" : "tab"} aria-current={route.view === target}>
      {label}
      {count != null && <span className="count">{count}</span>}
    </a>
  );

  function search(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const q = String(new FormData(e.currentTarget).get("q") ?? "").trim();
    window.location.hash = href("search", { q: q || undefined });
  }

  // While another view or search loads, the previous list stays out of sight.
  const current = data && data.data.view === route.view ? data.data : null;

  return (
    <>
      <header>
        <h1>velopim</h1>
        <nav>
          {tab("tasks", "Aufgaben", openCount)}
          {tab("products", "Produkte")}
          {tab("imports", "Importe")}
          {tab("pricing", "Kalkulation")}
        </nav>
        <form className="global-search" role="search" onSubmit={search}>
          <input
            type="search"
            name="q"
            key={route.view === "search" ? route.params.q : ""}
            defaultValue={route.view === "search" ? route.params.q : ""}
            placeholder="Alles durchsuchen"
            aria-label="Alles durchsuchen: Aufgaben, Produkte, Lieferantenartikel"
          />
        </form>
      </header>
      <main>
        {error && (
          <p className="error" role="alert">
            {error}{" "}
            <button type="button" onClick={() => void load()}>
              Erneut versuchen
            </button>
          </p>
        )}
        {current?.view === "tasks" && <Tasks list={current.tasks} params={route.params} onChanged={load} onError={setError} />}
        {current?.view === "products" && <Products list={current.products} params={route.params} />}
        {current?.view === "imports" && <ImportRuns runs={current.runs} params={route.params} />}
        {current?.view === "pricing" && (
          <Pricing rules={current.rules} params={route.params} onChanged={load} onError={setError} />
        )}
        {current?.view === "search" && <Search results={current.results} />}
      </main>
    </>
  );
}
