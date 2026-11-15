import { useCallback, useEffect, useState } from "react";
import {
  listImportRuns,
  listPricingRules,
  listProducts,
  listTasks,
  type ImportRun,
  type PricingRule,
  type Product,
  type TaskList,
} from "./api";
import { ImportRuns } from "./ImportRuns";
import { Pricing } from "./Pricing";
import { Products } from "./Products";
import { Tasks } from "./Tasks";

const views = ["open", "closed", "products", "imports", "pricing"] as const;
type View = (typeof views)[number];

function viewFromHash(): View {
  const hash = window.location.hash.slice(1) as View;
  return views.includes(hash) ? hash : "open";
}

type Data =
  | { view: "open" | "closed"; tasks: TaskList }
  | { view: "products"; products: Product[] }
  | { view: "imports"; runs: ImportRun[] }
  | { view: "pricing"; rules: PricingRule[] };

async function fetchData(view: View): Promise<Data> {
  switch (view) {
    case "products":
      return { view, products: await listProducts() };
    case "imports":
      return { view, runs: await listImportRuns() };
    case "pricing":
      return { view, rules: await listPricingRules() };
    default:
      return { view, tasks: await listTasks(view) };
  }
}

export function App() {
  const [view, setView] = useState<View>(viewFromHash);
  const [data, setData] = useState<Data | null>(null);
  const [openCount, setOpenCount] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const onHash = () => setView(viewFromHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  const load = useCallback(async () => {
    try {
      const next = await fetchData(view);
      setData(next);
      if (next.view === "open" || next.view === "closed") setOpenCount(next.tasks.open);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [view]);

  useEffect(() => {
    void load();
  }, [load]);

  const tab = (target: View, label: string, count?: number | null) => (
    <a href={`#${target}`} className={view === target ? "tab active" : "tab"} aria-current={view === target}>
      {label}
      {count != null && <span className="count">{count}</span>}
    </a>
  );

  // Data of another view is still on screen until the new one has loaded.
  const current = data?.view === view ? data : null;

  return (
    <>
      <header>
        <h1>velopim</h1>
        <nav>
          {tab("open", "Aufgaben", openCount)}
          {tab("closed", "Erledigt")}
          {tab("products", "Produkte")}
          {tab("imports", "Importe")}
          {tab("pricing", "Kalkulation")}
        </nav>
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
        {current && (current.view === "open" || current.view === "closed") && (
          <Tasks list={current.tasks} closed={current.view === "closed"} onChanged={load} onError={setError} />
        )}
        {current?.view === "products" && <Products products={current.products} />}
        {current?.view === "imports" && <ImportRuns runs={current.runs} />}
        {current?.view === "pricing" && <Pricing rules={current.rules} onChanged={load} onError={setError} />}
      </main>
    </>
  );
}
