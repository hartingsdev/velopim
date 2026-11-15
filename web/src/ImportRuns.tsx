import { useState, type FormEvent } from "react";
import { ApiError, uploadImport, type ImportList, type ImportResult } from "./api";
import { formatDate, runStatusLabel } from "./format";
import { href, matches, SearchBox, type Params } from "./route";

interface Props {
  list: ImportList;
  params: Params;
  onChanged: () => Promise<void>;
  onError: (message: string | null) => void;
}

function Upload({ list, onChanged, onError }: Omit<Props, "params">) {
  const [source, setSource] = useState(list.sources[0]?.id ?? "");
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<ImportResult | null>(null);
  const [problems, setProblems] = useState<string[]>([]);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!file || !source) return;
    const form = e.currentTarget;
    setBusy(true);
    setProblems([]);
    setResult(null);
    try {
      setResult(await uploadImport(source, file));
      onError(null);
      setFile(null);
      form.reset();
    } catch (err) {
      setProblems(err instanceof ApiError && err.issues.length > 0 ? err.issues : [(err as Error).message]);
    }
    await onChanged();
    setBusy(false);
  }

  if (list.sources.length === 0) {
    return <p className="hint">Noch kein Lieferant eingerichtet, daher kein Import möglich.</p>;
  }
  return (
    <form className="task" onSubmit={(e) => void submit(e)}>
      <h2>Lieferung importieren</h2>
      <div className="condition upload">
        <select value={source} onChange={(e) => setSource(e.target.value)} aria-label="Lieferant">
          {list.sources.map((s) => (
            <option key={s.id} value={s.id}>
              {s.name}
            </option>
          ))}
        </select>
        <input type="file" onChange={(e) => setFile(e.target.files?.[0] ?? null)} aria-label="Datei des Lieferanten" />
        <button type="submit" className="primary" disabled={busy || !file}>
          {busy ? "Importiert …" : "Importieren"}
        </button>
      </div>
      {problems.length > 0 && (
        <div className="error" role="alert">
          Die Datei wurde nicht importiert:
          <ul>
            {problems.map((p) => (
              <li key={p}>{p}</li>
            ))}
          </ul>
        </div>
      )}
      {result && (
        <p className={result.status === "held" ? "error" : "hint"} role="status">
          {result.status === "held"
            ? "Import angehalten: Es fehlen auffällig viele Artikel, deshalb wurde nichts deaktiviert. "
            : "Import abgeschlossen. "}
          {result.records} Artikel, {result.new} neu, {result.changed} geändert, {result.removed} entfallen, {result.issues}{" "}
          Datenprobleme. {result.auto_applied} automatisch übernommen,{" "}
          <a href={href("tasks")}>{result.pending_approval} zur Freigabe</a>.
        </p>
      )}
    </form>
  );
}

export function ImportRuns({ list, params, onChanged, onError }: Props) {
  const all = list.runs;
  const runs = all.filter((run) =>
    matches(params.q, run.source_name, run.source_id, `Lauf ${run.id}`, runStatusLabel(run.status), formatDate(run.started_at),
      ...(run.summary.issues ?? []).flatMap((i) => [i.key, i.message])),
  );
  return (
    <>
      <Upload list={list} onChanged={onChanged} onError={onError} />
      <div className="toolbar">
        <SearchBox view="imports" params={params} placeholder="Importe durchsuchen (Lieferant, Status, Datenproblem, Artikelnr.)" />
      </div>
      {runs.length === 0 && (
        <p className="empty">{all.length === 0 ? "Noch kein Import gelaufen." : "Kein Import passt zur Suche."}</p>
      )}
    <ul className="tasks">
      {runs.map((run) => {
        const s = run.summary;
        const issues = s.issues ?? [];
        return (
          <li key={run.id} className="task">
            <div className="task-head">
              <h2>
                {run.source_name} · Lauf {run.id}
              </h2>
              <span className="meta">
                <span className={`badge status-${run.status}`}>{runStatusLabel(run.status)}</span>
                {formatDate(run.started_at)}
              </span>
            </div>
            <p className="item">
              {run.record_count} Artikel · {s.new ?? 0} neu · {s.changed ?? 0} geändert · {s.removed ?? 0} entfallen ·{" "}
              {s.quantity_changes ?? 0} Mengenänderungen
            </p>
            {run.error && <p className="error">{run.error}</p>}
            {run.issue_count > 0 && (
              <details>
                <summary>{run.issue_count} Datenprobleme</summary>
                <ul className="issues">
                  {issues.map((issue, i) => (
                    <li key={i}>
                      {[issue.key, issue.field].filter(Boolean).join(" · ")}
                      {(issue.key || issue.field) && ": "}
                      {issue.message}
                    </li>
                  ))}
                </ul>
                {run.issue_count > issues.length && (
                  <p className="hint">
                    Angezeigt werden {issues.length} von {run.issue_count}.
                  </p>
                )}
              </details>
            )}
          </li>
        );
      })}
    </ul>
    </>
  );
}
