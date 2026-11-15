// Editor for the ordered rules that turn a supplier's raw fields into an
// availability status. Mirrors availability.MappingRule in Go.

export interface RuleCondition {
  field: string;
  op: "eq" | "contains" | "empty" | "not_empty" | "gt" | "lte";
  value?: string;
}

export interface MappingRule {
  name: string;
  when: RuleCondition[];
  status: string;
  lead_min_days?: number;
  lead_max_days?: number;
  quantity_field?: string;
  date_field?: string;
  date_layout?: string;
}

const statusLabels: Record<string, string> = {
  in_stock: "Auf Lager",
  orderable: "Bestellbar (mit Lieferzeit)",
  preorder: "Vorbestellbar (mit Termin)",
  on_request: "Auf Anfrage",
  unavailable: "Nicht verfügbar",
  discontinued: "Ausgelaufen",
};

const opLabels: Record<RuleCondition["op"], string> = {
  gt: "ist größer als",
  lte: "ist höchstens",
  eq: "ist gleich",
  contains: "enthält",
  not_empty: "ist ausgefüllt",
  empty: "ist leer",
};

const needsValue = (op: RuleCondition["op"]) => op !== "empty" && op !== "not_empty";

// Date layouts are Go layouts; these cover what supplier files use.
const dateLayouts = [
  { value: "02.01.2006", label: "31.12.2027" },
  { value: "2006-01-02", label: "2027-12-31" },
  { value: "02.01.06", label: "31.12.27" },
  { value: "01/02/2006", label: "12/31/2027" },
];

export const exampleRules: MappingRule[] = [
  { name: "auf Lager", when: [{ field: "Bestand", op: "gt", value: "0" }], status: "in_stock", quantity_field: "Bestand" },
  {
    name: "mit Termin",
    when: [{ field: "Liefertermin", op: "not_empty" }],
    status: "preorder",
    date_field: "Liefertermin",
    date_layout: "02.01.2006",
  },
  { name: "sonst", when: [], status: "unavailable" },
];

interface Props {
  rules: MappingRule[];
  onChange: (rules: MappingRule[]) => void;
  /** Id of a datalist with the file's column names. */
  columnList: string;
}

export function AvailabilityRules({ rules, onChange, columnList }: Props) {
  const update = (i: number, change: Partial<MappingRule>) => onChange(rules.map((r, j) => (j === i ? { ...r, ...change } : r)));
  const move = (i: number, by: number) => {
    const next = [...rules];
    const [rule] = next.splice(i, 1);
    next.splice(i + by, 0, rule);
    onChange(next);
  };
  const optionalNumber = (text: string) => (text.trim() === "" ? undefined : Math.max(0, Math.round(Number(text) || 0)));

  return (
    <div className="mapping">
      {rules.length === 0 && (
        <p className="empty">
          Noch keine Regel: Die Verfügbarkeit jedes Artikels bleibt unbekannt, und kein Rad ist verkaufbar.
        </p>
      )}
      {rules.map((rule, i) => (
        <fieldset key={i} className="split mapping-rule">
          <legend>
            Regel {i + 1}
            {i === 0 ? " (wird zuerst geprüft)" : ""}
          </legend>
          <div className="fields">
            <label>
              Name
              <input type="text" value={rule.name} onChange={(e) => update(i, { name: e.target.value })} placeholder="z. B. auf Lager" />
            </label>
            <label>
              Dann gilt
              <select value={rule.status} onChange={(e) => update(i, { status: e.target.value })}>
                {Object.entries(statusLabels).map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
            </label>
          </div>

          <p className="hint">{rule.when.length === 0 ? "Ohne Bedingung: gilt für jeden Artikel, den keine Regel davor erfasst." : "Wenn alle Bedingungen zutreffen:"}</p>
          {rule.when.map((c, k) => {
            const setCondition = (change: Partial<RuleCondition>) =>
              update(i, { when: rule.when.map((x, m) => (m === k ? { ...x, ...change } : x)) });
            return (
              <div key={k} className="condition">
                <input
                  type="text"
                  list={columnList}
                  value={c.field}
                  placeholder="Spalte"
                  aria-label="Spalte"
                  onChange={(e) => setCondition({ field: e.target.value })}
                />
                <select value={c.op} aria-label="Vergleich" onChange={(e) => setCondition({ op: e.target.value as RuleCondition["op"] })}>
                  {Object.entries(opLabels).map(([value, label]) => (
                    <option key={value} value={value}>
                      {label}
                    </option>
                  ))}
                </select>
                {needsValue(c.op) && (
                  <input type="text" value={c.value ?? ""} placeholder="Wert" aria-label="Wert" onChange={(e) => setCondition({ value: e.target.value })} />
                )}
                <button type="button" onClick={() => update(i, { when: rule.when.filter((_, m) => m !== k) })}>
                  Entfernen
                </button>
              </div>
            );
          })}
          <button type="button" onClick={() => update(i, { when: [...rule.when, { field: "", op: "gt", value: "0" }] })}>
            Bedingung hinzufügen
          </button>

          <div className="fields">
            <label>
              Spalte mit der Stückzahl (optional)
              <input
                type="text"
                list={columnList}
                value={rule.quantity_field ?? ""}
                onChange={(e) => update(i, { quantity_field: e.target.value || undefined })}
              />
            </label>
            <label>
              Lieferzeit von (Tage, optional)
              <input
                type="number"
                min="0"
                value={rule.lead_min_days ?? ""}
                onChange={(e) => update(i, { lead_min_days: optionalNumber(e.target.value) })}
              />
            </label>
            <label>
              Lieferzeit bis (Tage, optional)
              <input
                type="number"
                min="0"
                value={rule.lead_max_days ?? ""}
                onChange={(e) => update(i, { lead_max_days: optionalNumber(e.target.value) })}
              />
            </label>
            <label>
              Spalte mit dem Liefertermin (optional)
              <input
                type="text"
                list={columnList}
                value={rule.date_field ?? ""}
                onChange={(e) =>
                  update(i, { date_field: e.target.value || undefined, date_layout: e.target.value ? (rule.date_layout ?? dateLayouts[0].value) : undefined })
                }
              />
            </label>
            {rule.date_field && (
              <label>
                So steht das Datum in der Datei
                <select value={rule.date_layout ?? dateLayouts[0].value} onChange={(e) => update(i, { date_layout: e.target.value })}>
                  {dateLayouts.map((d) => (
                    <option key={d.value} value={d.value}>
                      {d.label}
                    </option>
                  ))}
                  {rule.date_layout && !dateLayouts.some((d) => d.value === rule.date_layout) && (
                    <option value={rule.date_layout}>{rule.date_layout}</option>
                  )}
                </select>
              </label>
            )}
          </div>

          <div className="decide">
            <button type="button" disabled={i === 0} onClick={() => move(i, -1)}>
              Nach oben
            </button>
            <button type="button" disabled={i === rules.length - 1} onClick={() => move(i, 1)}>
              Nach unten
            </button>
            <button type="button" onClick={() => onChange(rules.filter((_, j) => j !== i))}>
              Regel löschen
            </button>
          </div>
        </fieldset>
      ))}
      <div className="decide">
        <button type="button" onClick={() => onChange([...rules, { name: "", when: [], status: "unavailable" }])}>
          Regel hinzufügen
        </button>
        {rules.length === 0 && (
          <button type="button" onClick={() => onChange(exampleRules)}>
            Beispiel einsetzen
          </button>
        )}
      </div>
    </div>
  );
}
