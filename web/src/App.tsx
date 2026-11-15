import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  getProduct,
  getSyncPlan,
  listImportRuns,
  listPricingRules,
  listRules,
  listProducts,
  listTasks,
  listUsagePolicies,
  searchSupplierItems,
  type ImportRun,
  type PricingRule,
  type ProductDetail as ProductDetailData,
  type ProductList,
  type RuleList,
  type SyncPlan,
  type TaskList,
  type UsagePolicyList,
} from "./api";
import { ImportRuns } from "./ImportRuns";
import { Pricing } from "./Pricing";
import { ProductDetail } from "./ProductDetail";
import { Products } from "./Products";
import { Rules } from "./Rules";
import { href, useRoute, type Route, type View } from "./route";
import { Search, type SearchResults } from "./Search";
import { Sources } from "./Sources";
import { Sync } from "./Sync";
import { Tasks } from "./Tasks";

type Data =
  | { view: "tasks"; tasks: TaskList }
  | { view: "products"; products: ProductList }
  | { view: "imports"; runs: ImportRun[] }
  | { view: "pricing"; rules: PricingRule[] }
  | { view: "rules"; list: RuleList }
  | { view: "product"; detail: ProductDetailData }
  | { view: "sources"; list: UsagePolicyList }
  | { view: "sync"; plan: SyncPlan }
  | { view: "search"; results: SearchResults };

async function fetchData({ view, params }: Route): Promise<Data> {
  switch (view) {
    case "products":
      return { view, products: await listProducts({ status: params.status, unpriced: params.unpriced, q: params.q }) };
    case "imports":
      return { view, runs: await listImportRuns() };
    case "pricing":
      return { view, rules: await listPricingRules() };
    case "rules":
      return { view, list: await listRules() };
    case "product":
      return { view, detail: await getProduct(params.id ?? "") };
    case "sources":
      return { view, list: await listUsagePolicies() };
    case "sync":
      return { view, plan: await getSyncPlan() };
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
  const browserFiltered = route.view === "imports" || route.view === "pricing" || route.view === "rules";
  const serverSide = browserFiltered ? { view: route.view, params: {} } : route;
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

  // A single product belongs to the products tab.
  const isActive = (target: View) => route.view === target || (target === "products" && route.view === "product");
  const tab = (target: View, label: string, count?: number | null) => (
    <a href={href(target)} className={isActive(target) ? "tab active" : "tab"} aria-current={isActive(target)}>
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
          {tab("sync", "Shopify")}
          {tab("sources", "Lieferanten")}
          {tab("rules", "Regeln")}
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
        {current?.view === "rules" && <Rules list={current.list} params={route.params} onChanged={load} onError={setError} />}
        {current?.view === "product" && (
          <ProductDetail key={key} detail={current.detail} onChanged={load} onError={setError} />
        )}
        {current?.view === "sources" && <Sources list={current.list} onChanged={load} onError={setError} />}
        {current?.view === "sync" && <Sync plan={current.plan} />}
        {current?.view === "search" && <Search results={current.results} />}
      </main>
    </>
  );
}
