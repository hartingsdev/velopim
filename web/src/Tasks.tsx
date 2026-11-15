import { useState } from "react";
import { ApiError, confirmImportRun, decideTask, type Corrections, type Task, type TaskCategory, type TaskItem, type TaskList } from "./api";
import { Chip, href, SearchBox, type Params } from "./route";
import {
  axisLabel,
  categoryLabel,
  categoryOrder,
  fieldLabel,
  formatAmount,
  formatDate,
  formatPrice,
  formatValue,
  parseAmount,
  priceDelta,
  priorityLabel,
  reasonLabel,
} from "./format";

interface Props {
  list: TaskList;
  params: Params;
  onChanged: () => Promise<void>;
  onError: (message: string | null) => void;
}

export function Tasks({ list, params, onChanged, onError }: Props) {
  const closed = params.status === "closed";
  const filtered = Boolean(params.q || params.category);
  // What matches search and category; the list itself is capped.
  const matching = params.category
    ? (list.categories[params.category as TaskCategory] ?? 0)
    : Object.values(list.categories).reduce((sum, n) => sum + (n ?? 0), 0);
  const categories = categoryOrder.filter((c) => list.categories[c as TaskCategory] || c === params.category);
  return (
    <>
      <div className="toolbar">
        <div className="chips" role="group" aria-label="Status">
          <Chip view="tasks" params={{ ...params, status: undefined, category: undefined }} active={!closed}>
            Offen <span className="n">{list.open}</span>
          </Chip>
          <Chip view="tasks" params={{ ...params, status: "closed", category: undefined }} active={closed}>
            Erledigt <span className="n">{list.closed}</span>
          </Chip>
        </div>
        <SearchBox view="tasks" params={params} placeholder="Aufgaben durchsuchen (Titel, Artikelnr., EAN)" />
      </div>
      {categories.length > 0 && (
        <div className="chips" role="group" aria-label="Kategorie">
          <Chip view="tasks" params={{ ...params, category: undefined }} active={!params.category}>
            Alle
          </Chip>
          {categories.map((c) => (
            <Chip key={c} view="tasks" params={{ ...params, category: c }} active={params.category === c}>
              {categoryLabel(c)} <span className="n">{list.categories[c as TaskCategory] ?? 0}</span>
            </Chip>
          ))}
        </div>
      )}
      {list.tasks.length === 0 && (
        <p className="empty">
          {filtered ? "Keine Aufgabe passt zu Suche und Filter." : closed ? "Noch nichts erledigt." : "Keine offenen Aufgaben."}
        </p>
      )}
      {matching > list.tasks.length && (
        <p className="hint">
          Angezeigt werden {list.tasks.length} von {matching} Aufgaben. Grenze die Liste mit Suche oder Kategorie ein.
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

/** Turns the web addresses in a text into links. */
function linkify(text: string) {
  return text.split(/(https?:\/\/[^\s]+)/).map((part, i) =>
    /^https?:\/\//.test(part) ? (
      <a key={i} href={part} target="_blank" rel="noreferrer noopener">
        {part}
      </a>
    ) : (
      part
    ),
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
  // Prices as typed, keyed by supplier article number; "" is the single
  // price of a price-change task. Untouched fields are not in here.
  const [prices, setPrices] = useState<Record<string, string>>({});
  const open = task.status === "open";
  const isApproval = task.action !== null || task.kind === "publish_product";
  const isModel = task.variants.length > 0;

  /** Collects what was changed in the form; null when a price is not a number. */
  function corrections(): Corrections | null {
    const out: Corrections = {};
    const invalid: string[] = [];
    if (isModel) {
      out.split_by = splitBy;
      const variantPrices: Record<string, number> = {};
      for (const v of task.variants) {
        const typed = prices[v.sku];
        if (typed === undefined || typed.trim() === "") continue;
        const cents = parseAmount(typed);
        if (cents === null || cents <= 0) invalid.push(`${v.sku}: „${typed}“ ist kein Preis`);
        else if (cents !== v.price_cents) variantPrices[v.sku] = cents;
      }
      if (Object.keys(variantPrices).length > 0) out.variant_prices = variantPrices;
    } else if (prices[""] !== undefined && prices[""].trim() !== "") {
      const cents = parseAmount(prices[""]);
      if (cents === null || cents <= 0) invalid.push(`„${prices[""]}“ ist kein Preis`);
      else if (cents !== task.action?.proposed_price_cents) out.price_cents = cents;
    }
    if (invalid.length > 0) {
      setIssues(invalid);
      return null;
    }
    return out;
  }

  async function decide(approve: boolean) {
    setIssues([]);
    const changed = approve ? corrections() : {};
    if (changed === null) return;
    setBusy(true);
    try {
      await decideTask(task.id, approve, note.trim(), changed);
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

  /** The delivery was complete after all: deactivate what is missing from it. */
  async function confirmHeld() {
    setBusy(true);
    try {
      await confirmImportRun(task.import_run_id!);
      onError(null);
    } catch (e) {
      const outdated = e instanceof ApiError && e.status === 409;
      onError(
        outdated
          ? "Zu diesem Lieferanten gibt es schon einen neueren Import. Diese Lieferung lässt sich nicht mehr bestätigen; schließe die Aufgabe mit „Nichts deaktivieren“."
          : (e as Error).message,
      );
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
          <span className="badge status-done">{categoryLabel(task.category)}</span>
          {open && task.priority === 1 && <span className="badge">{priorityLabel(1)}</span>}
          {formatDate(task.created_at)}
        </span>
      </div>
      {itemFacts.length > 0 && <p className="item">{itemFacts.join(" · ")}</p>}
      {task.detail && <p className="detail">{linkify(task.detail)}</p>}
      {task.product_id !== null && (
        <p className="item">
          <a href={href("product", { id: String(task.product_id) })}>Produkt ansehen</a>
        </p>
      )}
      {isModel && (
        <Variants
          variants={task.variants}
          prices={open ? prices : null}
          onPrice={(sku, value) => setPrices({ ...prices, [sku]: value })}
        />
      )}
      {isModel && open && (
        <Split variants={task.variants} locked={task.locked_split_by} splitBy={splitBy} onChange={setSplitBy} />
      )}
      {task.action && (
        <Change
          action={task.action}
          price={open ? (prices[""] ?? formatAmount(task.action.proposed_price_cents)) : null}
          onPrice={(value) => setPrices({ ...prices, "": value })}
        />
      )}
      {issues.length > 0 && (
        <div className="error" role="alert">
          So geht die Freigabe nicht:
          <ul>
            {issues.map((issue) => (
              <li key={issue}>{issue}</li>
            ))}
          </ul>
          {isModel && issues.some((i) => i.includes("unterscheidbar")) &&
            "Wähle ein Trennmerkmal, das diese Artikel unterscheidet, oder lehne ab."}
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
          ) : task.kind === "import_held" && task.import_run_id !== null ? (
            <>
              <button type="button" className="primary" disabled={busy} onClick={() => void confirmHeld()}>
                Fehlende Artikel deaktivieren
              </button>
              <button type="button" disabled={busy} onClick={() => void decide(false)}>
                Nichts deaktivieren
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
  if (task.action || task.kind === "publish_product") return task.status === "done" ? "Freigegeben" : "Abgelehnt";
  return task.status === "done" ? "Erledigt" : "Verworfen";
}

function Change(props: { action: NonNullable<Task["action"]>; price: string | null; onPrice: (value: string) => void }) {
  const { action, price, onPrice } = props;
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
      {action.base_price_cents !== null && action.proposed_price_cents !== null && (
        <p className="values">
          <span className="field">Dein Verkaufspreis bei Freigabe</span>
          <span className="old">{formatPrice(action.base_price_cents)}</span>
          <span aria-label="wird zu">→</span>
          {price === null ? (
            <span className="new">{formatPrice(action.proposed_price_cents)}</span>
          ) : (
            <span className="amount">
              <input
                type="text"
                inputMode="decimal"
                value={price}
                onChange={(e) => onPrice(e.target.value)}
                aria-label="Verkaufspreis bei Freigabe in Euro"
              />{" "}
              €
            </span>
          )}
          <span className="delta">
            {priceDelta(String(action.base_price_cents), String(parseAmount(price ?? "") ?? action.proposed_price_cents))}
          </span>
        </p>
      )}
      {action.field === "cost_cents" && action.proposed_price_cents === null && (
        <p className="hint">Kein Produkt mit Verkaufspreis zu diesem Artikel: Die Freigabe ändert keinen Preis.</p>
      )}
      <p className="reason">
        {reasonLabel(action.reason)}
        {action.rule && <> (Regel: {action.rule})</>}
      </p>
    </div>
  );
}

function Variants(props: {
  variants: TaskItem[];
  /** Prices as typed; null when the task is closed and nothing can be edited. */
  prices: Record<string, string> | null;
  onPrice: (sku: string, value: string) => void;
}) {
  const { variants, prices, onPrice } = props;
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
                <td className="num">
                  {prices === null ? (
                    formatPrice(v.price_cents)
                  ) : (
                    <span className="amount">
                      <input
                        type="text"
                        inputMode="decimal"
                        value={prices[v.sku] ?? formatAmount(v.price_cents)}
                        onChange={(e) => onPrice(v.sku, e.target.value)}
                        aria-label={`Verkaufspreis für ${v.sku} in Euro`}
                      />{" "}
                      €
                    </span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {unpriced && (
        <p className="hint">
          Ohne passende Regel unter <a href="#pricing">Kalkulation</a> und ohne eingetragenen Preis werden Varianten
          ohne Verkaufspreis angelegt.
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
