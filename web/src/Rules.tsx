import { useState, type FormEvent } from "react";
import { ApiError, deleteRule, saveRule, type Rule, type RuleActionType, type RuleCondition, type RuleList } from "./api";
import { formatAmount, parseAmount } from "./format";
import { matches, SearchBox, type Params } from "./route";

interface Props {
  list: RuleList;
  params: Params;
  onChanged: () => Promise<void>;
  onError: (message: string | null) => void;
}

const actionLabels: Record<RuleActionType, string> = {
  price_change: "Preisänderung",
  new_product: "Neues Modell",
  availability_status_change: "Verfügbarkeit",
  deactivation: "Artikel entfällt",
  description_change: "Text",
  images_change: "Bilder",
  attribute_change: "Merkmale (EAN, Farbe …)",
};

const factLabels: Record<RuleCondition["fact"], string> = {
  cost_change_cents: "Änderung des EK in €",
  margin_pct: "Marge nach Neukalkulation in %",
  price_change_abs_pct: "Preisänderung nach Neukalkulation in % (Betrag)",
  price_change_pct: "Preisänderung nach Neukalkulation in % (mit Vorzeichen)",
};

const opLabels: Record<RuleCondition["op"], string> = { lt: "kleiner als", lte: "höchstens", gt: "größer als", gte: "mindestens" };

const empty: Rule = { id: 0, name: "", enabled: true, action_type: "price_change", scope: {}, conditions: [], outcome: "approval", guard: false };

/** Ready-made rules for the common cases. */
const presets: { label: string; rule: Rule }[] = [
  {
    label: "EK-Senkungen automatisch weitergeben",
    rule: { ...empty, name: "EK-Senkungen automatisch weitergeben", outcome: "auto", conditions: [{ fact: "cost_change_cents", op: "lt", value: 0 }] },
  },
  {
    label: "EK-Erhöhungen automatisch weitergeben",
    rule: { ...empty, name: "EK-Erhöhungen automatisch weitergeben", outcome: "auto", conditions: [{ fact: "cost_change_cents", op: "gt", value: 0 }] },
  },
  { label: "Bilder automatisch übernehmen", rule: { ...empty, name: "Bilder automatisch übernehmen", action_type: "images_change", outcome: "auto" } },
  {
    label: "Verfügbarkeit ohne Aufgabe",
    rule: { ...empty, name: "Verfügbarkeit ohne Aufgabe", action_type: "availability_status_change", outcome: "auto" },
  },
];

// The purchase price fact is stored in cents and edited in euros.
function conditionText(c: RuleCondition): string {
  const value = c.fact === "cost_change_cents" ? formatAmount(c.value) : c.value.toLocaleString("de-DE");
  return `${factLabels[c.fact] ?? c.fact} ${opLabels[c.op] ?? c.op} ${value}`;
}

function describe(rule: Rule, sources: RuleList["sources"]): string {
  const scope = [
    rule.scope.source_id ? (sources.find((s) => s.id === rule.scope.source_id)?.name ?? rule.scope.source_id) : "alle Lieferanten",
    rule.scope.brand || "alle Marken",
    rule.scope.category || "alle Kategorien",
  ].join(", ");
  const when = rule.conditions.length > 0 ? `, wenn ${rule.conditions.map(conditionText).join(" und ")}` : "";
  const outcome = rule.outcome === "auto" ? "automatisch übernehmen" : rule.guard ? "immer Freigabe (Schutzregel)" : "Freigabe";
  return `${actionLabels[rule.action_type] ?? rule.action_type} · ${scope}${when} → ${outcome}`;
}

export function Rules({ list, params, onChanged, onError }: Props) {
  const [draft, setDraft] = useState<Rule>(empty);
  // Condition values as typed, parallel to draft.conditions.
  const [values, setValues] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);

  const rules = list.rules.filter((r) => matches(params.q, r.name, describe(r, list.sources)));

  function edit(rule: Rule) {
    setDraft({ ...rule, scope: { ...rule.scope }, conditions: rule.conditions.map((c) => ({ ...c })) });
    setValues(rule.conditions.map((c) => (c.fact === "cost_change_cents" ? formatAmount(c.value) : String(c.value).replace(".", ","))));
    setProblem(null);
  }

  function setCondition(i: number, change: Partial<RuleCondition>) {
    setDraft({ ...draft, conditions: draft.conditions.map((c, j) => (j === i ? { ...c, ...change } : c)) });
  }

  async function save(e: FormEvent) {
    e.preventDefault();
    const conditions: RuleCondition[] = [];
    for (const [i, c] of draft.conditions.entries()) {
      const text = (values[i] ?? "").trim();
      const negative = text.startsWith("-") || text.startsWith("−");
      const unsigned = text.replace(/^[-−]/, "");
      const number = c.fact === "cost_change_cents" ? parseAmount(unsigned) : Number(unsigned.replace(",", "."));
      if (number === null || text === "" || Number.isNaN(number)) {
        setProblem(`Bedingung ${i + 1}: „${text}“ ist keine Zahl.`);
        return;
      }
      conditions.push({ ...c, value: negative ? -number : number });
    }
    setBusy(true);
    setProblem(null);
    try {
      await saveRule({ ...draft, name: draft.name.trim(), conditions, guard: draft.outcome === "approval" && draft.guard });
      onError(null);
      edit(empty);
      await onChanged();
    } catch (err) {
      setProblem((err as Error).message);
    }
    setBusy(false);
  }

  async function toggle(rule: Rule) {
    try {
      await saveRule({ ...rule, enabled: !rule.enabled });
      onError(null);
    } catch (err) {
      onError((err as Error).message);
    }
    await onChanged();
  }

  async function remove(rule: Rule) {
    try {
      await deleteRule(rule.id);
      onError(null);
    } catch (err) {
      const used = err instanceof ApiError && err.status === 409;
      onError(used ? "Die Regel hat schon entschieden und bleibt fürs Protokoll erhalten. Schalte sie stattdessen aus." : (err as Error).message);
    }
    await onChanged();
  }

  const priceRule = draft.action_type === "price_change";

  return (
    <>
      <p className="hint">
        Regeln entscheiden, ob eine Änderung vom Lieferanten automatisch übernommen wird oder als Aufgabe zur Freigabe
        kommt. Die genaueste passende Regel gilt (Lieferant, Marke, Kategorie). Passt keine Regel oder widersprechen sich
        zwei gleich genaue, gilt Freigabe. Eine Schutzregel erzwingt die Freigabe gegen alle anderen.
      </p>
      <div className="toolbar">
        <SearchBox view="rules" params={params} placeholder="Regeln durchsuchen" />
      </div>
      {rules.length === 0 && (
        <p className="empty">
          {list.rules.length === 0 ? "Noch keine Regel: Jede Änderung kommt als Aufgabe zur Freigabe." : "Keine Regel passt zur Suche."}
        </p>
      )}
      <ul className="tasks">
        {rules.map((rule) => (
          <li key={rule.id} className={rule.enabled ? "task" : "task disabled"}>
            <div className="task-head">
              <h2>{rule.name}</h2>
              <span className="meta">
                {!rule.enabled && <span className="badge status-done">Aus</span>}
                <button type="button" onClick={() => void toggle(rule)}>
                  {rule.enabled ? "Ausschalten" : "Einschalten"}
                </button>
                <button type="button" onClick={() => edit(rule)}>
                  Bearbeiten
                </button>
                <button type="button" onClick={() => void remove(rule)}>
                  Löschen
                </button>
              </span>
            </div>
            <p className="item">{describe(rule, list.sources)}</p>
          </li>
        ))}
      </ul>

      <form className="task rule-form" onSubmit={(e) => void save(e)}>
        <h2>{draft.id ? "Regel ändern" : "Neue Regel"}</h2>
        {!draft.id && (
          <div className="chips presets">
            {presets.map((p) => (
              <button key={p.label} type="button" className="chip" onClick={() => edit(p.rule)}>
                {p.label}
              </button>
            ))}
          </div>
        )}
        <div className="fields">
          <label className="wide">
            Name
            <input type="text" required value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
          </label>
          <label>
            Gilt für
            <select
              value={draft.action_type}
              onChange={(e) => {
                const action_type = e.target.value as RuleActionType;
                // Conditions exist only for price changes.
                setDraft({ ...draft, action_type, conditions: action_type === "price_change" ? draft.conditions : [] });
              }}
            >
              {Object.entries(actionLabels).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </label>
          <label>
            Lieferant
            <select
              value={draft.scope.source_id ?? ""}
              onChange={(e) => setDraft({ ...draft, scope: { ...draft.scope, source_id: e.target.value } })}
            >
              <option value="">alle</option>
              {list.sources.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Marke
            <input
              type="text"
              placeholder="alle"
              value={draft.scope.brand ?? ""}
              onChange={(e) => setDraft({ ...draft, scope: { ...draft.scope, brand: e.target.value.trim() } })}
            />
          </label>
          <label>
            Kategorie
            <input
              type="text"
              placeholder="alle"
              value={draft.scope.category ?? ""}
              onChange={(e) => setDraft({ ...draft, scope: { ...draft.scope, category: e.target.value.trim() } })}
            />
          </label>
          <label>
            Ergebnis
            <select value={draft.outcome} onChange={(e) => setDraft({ ...draft, outcome: e.target.value as Rule["outcome"] })}>
              <option value="approval">Freigabe</option>
              <option value="auto">automatisch übernehmen</option>
            </select>
          </label>
        </div>

        {priceRule && (
          <div className="conditions">
            {draft.conditions.map((c, i) => (
              <div key={i} className="condition">
                <select value={c.fact} onChange={(e) => setCondition(i, { fact: e.target.value as RuleCondition["fact"] })} aria-label="Wert">
                  {Object.entries(factLabels).map(([value, label]) => (
                    <option key={value} value={value}>
                      {label}
                    </option>
                  ))}
                </select>
                <select value={c.op} onChange={(e) => setCondition(i, { op: e.target.value as RuleCondition["op"] })} aria-label="Vergleich">
                  {Object.entries(opLabels).map(([value, label]) => (
                    <option key={value} value={value}>
                      {label}
                    </option>
                  ))}
                </select>
                <input
                  type="text"
                  inputMode="decimal"
                  value={values[i] ?? ""}
                  onChange={(e) => setValues(draft.conditions.map((_, j) => (j === i ? e.target.value : (values[j] ?? ""))))}
                  aria-label="Zahl"
                />
                <button
                  type="button"
                  onClick={() => {
                    setDraft({ ...draft, conditions: draft.conditions.filter((_, j) => j !== i) });
                    setValues(values.filter((_, j) => j !== i));
                  }}
                >
                  Entfernen
                </button>
              </div>
            ))}
            <button
              type="button"
              onClick={() => {
                setDraft({ ...draft, conditions: [...draft.conditions, { fact: "cost_change_cents", op: "lt", value: 0 }] });
                setValues([...draft.conditions.map((_, j) => values[j] ?? ""), "0,00"]);
              }}
            >
              Bedingung hinzufügen
            </button>
            <p className="hint">
              Der EK sinkt: „Änderung des EK kleiner als 0,00“. Marge und Preisänderung in % beziehen sich auf den Preis,
              den die Kalkulation neu errechnen würde, und brauchen eine passende Kalkulationsregel.
            </p>
          </div>
        )}

        {draft.outcome === "approval" && (
          <label className="check">
            <input type="checkbox" checked={draft.guard} onChange={(e) => setDraft({ ...draft, guard: e.target.checked })} />
            Schutzregel: erzwingt die Freigabe auch gegen genauere Regeln
          </label>
        )}
        <label className="check">
          <input type="checkbox" checked={draft.enabled} onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })} />
          Eingeschaltet
        </label>
        {problem && (
          <p className="error" role="alert">
            {problem}
          </p>
        )}
        <div className="decide">
          <button type="submit" className="primary" disabled={busy}>
            {draft.id ? "Regel ändern" : "Regel anlegen"}
          </button>
          {(draft.id !== 0 || draft.name !== "") && (
            <button type="button" onClick={() => edit(empty)}>
              Abbrechen
            </button>
          )}
        </div>
      </form>
    </>
  );
}
