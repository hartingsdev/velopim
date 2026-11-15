import { useState } from "react";
import { ApiError, decideTask, type Task, type TaskItem, type TaskList } from "./api";
import { axisLabel, fieldLabel, formatDate, formatPrice, formatValue, priceDelta, priorityLabel, reasonLabel } from "./format";

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

function axisValue(item: TaskItem, axis: string): string {
  if (axis === "color") return item.color.trim();
  if (axis === "frame_size") return item.frame_size.trim();
  return (item.attributes?.[axis.slice("attributes.".length)] ?? "").trim();
}

/** Axes in which the variants of a model actually differ. */
function splitAxes(variants: TaskItem[]): string[] {
  const axes = ["color", "frame_size"];
  const keys = new Set<string>();
  for (const v of variants) for (const k of Object.keys(v.attributes ?? {})) keys.add(k);
  for (const k of [...keys].sort()) axes.push(`attributes.${k}`);
  return axes.filter((axis) => new Set(variants.map((v) => axisValue(v, axis))).size > 1);
}

/** Titles of the products a model becomes when split by the given axes. */
function productTitles(variants: TaskItem[], splitBy: string[]): Map<string, number> {
  const titles = new Map<string, number>();
  for (const v of variants) {
    const title = [v.name, ...splitBy.map((axis) => axisValue(v, axis))].filter(Boolean).join(" ");
    titles.set(title, (titles.get(title) ?? 0) + 1);
  }
  return titles;
}

function TaskCard({ task, onChanged, onError }: { task: Task } & Pick<Props, "onChanged" | "onError">) {
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [splitBy, setSplitBy] = useState<string[]>([]);
  const [issues, setIssues] = useState<string[]>([]);
  const open = task.status === "open";
  const isApproval = task.action !== null;
  const isModel = task.variants.length > 0;

  async function decide(approve: boolean) {
    setBusy(true);
    setIssues([]);
    try {
      await decideTask(task.id, approve, note.trim(), approve && isModel ? splitBy : []);
      onError(null);
    } catch (e) {
      if (e instanceof ApiError && e.status === 422) {
        // The model cannot be created as cut; the task stays open.
        setIssues(e.issues.length > 0 ? e.issues : [e.message]);
        setBusy(false);
        return;
      }
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
      {isModel && <Variants variants={task.variants} />}
      {isModel && open && (
        <Split variants={task.variants} locked={task.locked_split_by} splitBy={splitBy} onChange={setSplitBy} />
      )}
      {task.action && <Change action={task.action} />}
      {issues.length > 0 && (
        <div className="error" role="alert">
          So lässt sich das Modell nicht anlegen:
          <ul>
            {issues.map((issue) => (
              <li key={issue}>{issue}</li>
            ))}
          </ul>
          Wähle ein Trennmerkmal, das diese Artikel unterscheidet, oder lehne ab.
        </div>
      )}

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

function Variants({ variants }: { variants: TaskItem[] }) {
  const attributeKeys = [...new Set(variants.flatMap((v) => Object.keys(v.attributes ?? {})))].sort();
  const unpriced = variants.some((v) => v.price_cents === null);
  return (
    <details className="variants" open={variants.length <= 12}>
      <summary>
        {variants.length === 1 ? "1 Variante" : `${variants.length} Varianten`} · {variants[0].source_id}
      </summary>
      <div className="scroll">
        <table>
          <thead>
            <tr>
              <th>Artikelnr.</th>
              <th>Farbe</th>
              <th>Rahmenhöhe</th>
              {attributeKeys.map((k) => (
                <th key={k}>{k}</th>
              ))}
              <th className="num">EK</th>
              <th className="num">UVP</th>
              <th className="num">Verkaufspreis</th>
            </tr>
          </thead>
          <tbody>
            {variants.map((v) => (
              <tr key={v.sku}>
                <td>{v.sku}</td>
                <td>{v.color || "–"}</td>
                <td>{v.frame_size || "–"}</td>
                {attributeKeys.map((k) => (
                  <td key={k}>{v.attributes?.[k] || "–"}</td>
                ))}
                <td className="num">{formatPrice(v.cost_cents)}</td>
                <td className="num">{formatPrice(v.rrp_cents)}</td>
                <td className="num">{formatPrice(v.price_cents)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {unpriced && (
        <p className="hint">
          Ohne passende Regel unter <a href="#pricing">Kalkulation</a> werden Varianten ohne Verkaufspreis angelegt.
        </p>
      )}
    </details>
  );
}

function Split(props: { variants: TaskItem[]; locked: string[] | null; splitBy: string[]; onChange: (axes: string[]) => void }) {
  const { variants, locked, splitBy, onChange } = props;
  if (locked) {
    return (
      <p className="hint">
        {locked.length > 0
          ? `Das Modell ist schon nach ${locked.map(axisLabel).join(", ")} getrennt. Die neuen Varianten kommen zu den passenden Produkten.`
          : "Das Modell ist schon ein Produkt. Die neuen Varianten kommen dazu."}
      </p>
    );
  }
  const axes = splitAxes(variants);
  if (axes.length === 0) return null;
  const titles = [...productTitles(variants, splitBy)];
  return (
    <fieldset className="split">
      <legend>Eigene Produkte je</legend>
      <div className="axes">
        {axes.map((axis) => (
          <label key={axis}>
            <input
              type="checkbox"
              checked={splitBy.includes(axis)}
              onChange={(e) => onChange(e.target.checked ? [...splitBy, axis] : splitBy.filter((a) => a !== axis))}
            />
            {axisLabel(axis)}
          </label>
        ))}
      </div>
      <p className="hint">
        {titles.length === 1 ? "Ergibt ein Produkt: " : `Ergibt ${titles.length} Produkte: `}
        {titles.map(([title, n]) => `${title} (${n})`).join(" · ")}
      </p>
    </fieldset>
  );
}
