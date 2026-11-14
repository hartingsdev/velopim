import type { ImportRun } from "./api";
import { formatDate, runStatusLabel } from "./format";

export function ImportRuns({ runs }: { runs: ImportRun[] }) {
  if (runs.length === 0) {
    return <p className="empty">Noch kein Import gelaufen.</p>;
  }
  return (
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
  );
}
