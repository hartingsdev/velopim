import { useState } from "react";
import { ApiError, decideTask, type Task, type TaskList } from "./api";
import { fieldLabel, formatDate, formatValue, priceDelta, priorityLabel, reasonLabel } from "./format";

interface Props {
  list: TaskList;
  closed: boolean;
  onChanged: () => Promise<void>;
  onError: (message: string | null) => void;
}

export function Tasks({ list, closed, onChanged, onError }: Props) {
  const total = closed ? list.closed : list.open;
  if (list.tasks.length === 0) {
    return <p className="empty">{closed ? "Noch nichts erledigt." : "Keine offenen Aufgaben."}</p>;
  }
  return (
    <>
      {total > list.tasks.length && (
        <p className="hint">
          Angezeigt werden {list.tasks.length} von {total} Aufgaben.
        </p>
      )}
      <ul className="tasks">
        {list.tasks.map((task) => (
          <TaskCard key={task.id} task={task} onChanged={onChanged} onError={onError} />
        ))}
      </ul>
    </>
  );
}

function TaskCard({ task, onChanged, onError }: { task: Task } & Pick<Props, "onChanged" | "onError">) {
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const open = task.status === "open";
  const isApproval = task.action !== null;

  async function decide(approve: boolean) {
    setBusy(true);
    try {
      await decideTask(task.id, approve, note.trim());
      onError(null);
    } catch (e) {
      // 409: someone or something closed the task in the meantime.
      const gone = e instanceof ApiError && e.status === 409;
      onError(gone ? "Die Aufgabe war schon geschlossen. Die Liste wurde aktualisiert." : (e as Error).message);
    }
    await onChanged();
    setBusy(false);
  }

  const item = task.item;
  const itemFacts = item
    ? [item.sku, item.model_year, item.color, item.frame_size && `RH ${item.frame_size}`, item.source_id].filter(Boolean)
    : [];

  return (
    <li className={`task prio-${task.priority}`}>
      <div className="task-head">
        <h2>{task.title}</h2>
        <span className="meta">
          {open && task.priority === 1 && <span className="badge">{priorityLabel(1)}</span>}
          {formatDate(task.created_at)}
        </span>
      </div>
      {itemFacts.length > 0 && <p className="item">{itemFacts.join(" · ")}</p>}
      {task.detail && <p>{task.detail}</p>}
      {task.action && <Change action={task.action} />}

      {open ? (
        <div className="decide">
          <input
            type="text"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder="Notiz (optional)"
            aria-label="Notiz zur Entscheidung"
            maxLength={500}
          />
          {isApproval ? (
            <>
              <button type="button" className="primary" disabled={busy} onClick={() => void decide(true)}>
                Freigeben
              </button>
              <button type="button" disabled={busy} onClick={() => void decide(false)}>
                Ablehnen
              </button>
            </>
          ) : (
            <button type="button" className="primary" disabled={busy} onClick={() => void decide(true)}>
              Erledigt
            </button>
          )}
        </div>
      ) : (
        <p className="outcome">
          {outcomeLabel(task)} von {task.closed_by ?? "unbekannt"} am {formatDate(task.closed_at)}
          {task.note && <> – {task.note}</>}
        </p>
      )}
    </li>
  );
}

function outcomeLabel(task: Task): string {
  if (task.action) return task.status === "done" ? "Freigegeben" : "Abgelehnt";
  return task.status === "done" ? "Erledigt" : "Verworfen";
}

function Change({ action }: { action: NonNullable<Task["action"]> }) {
  const delta = action.field.endsWith("_cents") ? priceDelta(action.old, action.new) : null;
  return (
    <div className="change">
      {action.field && (
        <p className="values">
          <span className="field">{fieldLabel(action.field)}</span>
          <span className="old">{formatValue(action.field, action.old)}</span>
          <span aria-label="wird zu">→</span>
          <span className="new">{formatValue(action.field, action.new)}</span>
          {delta && <span className="delta">{delta}</span>}
        </p>
      )}
      <p className="reason">
        {reasonLabel(action.reason)}
        {action.rule && <> (Regel: {action.rule})</>}
      </p>
    </div>
  );
}
