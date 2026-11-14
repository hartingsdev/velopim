import { useCallback, useEffect, useState } from "react";
import { listImportRuns, listTasks, type ImportRun, type TaskList } from "./api";
import { ImportRuns } from "./ImportRuns";
import { Tasks } from "./Tasks";

type View = "open" | "closed" | "imports";

function viewFromHash(): View {
  const hash = window.location.hash.slice(1);
  return hash === "closed" || hash === "imports" ? hash : "open";
}

export function App() {
  const [view, setView] = useState<View>(viewFromHash);
  const [tasks, setTasks] = useState<TaskList | null>(null);
  const [runs, setRuns] = useState<ImportRun[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const onHash = () => setView(viewFromHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  const load = useCallback(async () => {
    try {
      if (view === "imports") {
        setRuns(await listImportRuns());
      } else {
        setTasks(await listTasks(view));
      }
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [view]);

  useEffect(() => {
    setTasks(null);
    void load();
  }, [load]);

  const tab = (target: View, label: string, count?: number) => (
    <a href={`#${target}`} className={view === target ? "tab active" : "tab"} aria-current={view === target}>
      {label}
      {count !== undefined && <span className="count">{count}</span>}
    </a>
  );

  return (
    <>
      <header>
        <h1>velopim</h1>
        <nav>
          {tab("open", "Aufgaben", tasks?.open)}
          {tab("closed", "Erledigt")}
          {tab("imports", "Importe")}
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
        {view === "imports" ? (
          runs && <ImportRuns runs={runs} />
        ) : (
          tasks && <Tasks list={tasks} closed={view === "closed"} onChanged={load} onError={setError} />
        )}
      </main>
    </>
  );
}
