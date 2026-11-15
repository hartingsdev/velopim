import { useState, type FormEvent } from "react";
import { deletePricingRule, savePricingRule, type PricingConfig, type PricingRule } from "./api";

interface Props {
  rules: PricingRule[];
  onChanged: () => Promise<void>;
  onError: (message: string | null) => void;
}

const roundingLabels: Record<PricingConfig["rounding"], string> = {
  none: "auf den Cent genau",
  whole_euro: "auf volle Euro",
  end_99: "auf ,99",
  end_9: "auf 9 Euro (z. B. 1.239 €)",
};

const emptyConfig: PricingConfig = { mode: "markup", value: 1.6, vat_pct: 19, rounding: "end_99", cap_at_rrp: true };

function describe(c: PricingConfig): string {
  const value = c.value.toLocaleString("de-DE");
  const base = c.mode === "markup" ? `EK × ${value}` : `Zielmarge ${value} %`;
  const parts = [base, `zzgl. ${c.vat_pct.toLocaleString("de-DE")} % MwSt.`, `gerundet ${roundingLabels[c.rounding] ?? c.rounding}`];
  if (c.cap_at_rrp) parts.push("höchstens UVP");
  return parts.join(", ");
}

export function Pricing({ rules, onChanged, onError }: Props) {
  const [brand, setBrand] = useState("");
  const [category, setCategory] = useState("");
  const [config, setConfig] = useState<PricingConfig>(emptyConfig);
  const [busy, setBusy] = useState(false);

  const existing = rules.find((r) => r.brand === brand.trim() && r.category === category.trim());

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      await savePricingRule(brand.trim(), category.trim(), config);
      onError(null);
      await onChanged();
    } catch (err) {
      onError((err as Error).message);
    }
    setBusy(false);
  }

  async function remove(rule: PricingRule) {
    try {
      await deletePricingRule(rule.id);
      onError(null);
    } catch (err) {
      onError((err as Error).message);
    }
    await onChanged();
  }

  function edit(rule: PricingRule) {
    setBrand(rule.brand);
    setCategory(rule.category);
    setConfig({ ...emptyConfig, ...rule.config, rounding: rule.config.rounding || "none" });
  }

  const number = (value: string) => (value === "" ? 0 : Number(value));

  return (
    <>
      <p className="hint">
        Der Verkaufspreis wird beim Anlegen einer Variante aus dem Einkaufspreis berechnet. Es gilt die genaueste Regel:
        Marke und Kategorie, dann Marke, dann Kategorie, dann die Regel für alle. Bestehende Preise ändern sich dadurch
        nicht.
      </p>
      {rules.length === 0 ? (
        <p className="empty">Noch keine Regel. Ohne Regel werden Varianten ohne Verkaufspreis angelegt.</p>
      ) : (
        <ul className="tasks">
          {rules.map((rule) => (
            <li key={rule.id} className="task">
              <div className="task-head">
                <h2>{[rule.brand || "Alle Marken", rule.category || "alle Kategorien"].join(" · ")}</h2>
                <span className="meta">
                  <button type="button" onClick={() => edit(rule)}>
                    Bearbeiten
                  </button>
                  <button type="button" onClick={() => void remove(rule)}>
                    Löschen
                  </button>
                </span>
              </div>
              <p className="item">{describe(rule.config)}</p>
            </li>
          ))}
        </ul>
      )}

      <form className="task rule-form" onSubmit={(e) => void save(e)}>
        <h2>{existing ? "Regel ändern" : "Neue Regel"}</h2>
        <div className="fields">
          <label>
            Marke
            <input type="text" value={brand} onChange={(e) => setBrand(e.target.value)} placeholder="alle" />
          </label>
          <label>
            Kategorie
            <input type="text" value={category} onChange={(e) => setCategory(e.target.value)} placeholder="alle" />
          </label>
          <label>
            Verfahren
            <select
              value={config.mode}
              onChange={(e) => setConfig({ ...config, mode: e.target.value as PricingConfig["mode"] })}
            >
              <option value="markup">Aufschlag auf EK (Faktor)</option>
              <option value="target_margin">Zielmarge (%)</option>
            </select>
          </label>
          <label>
            {config.mode === "markup" ? "Faktor" : "Marge in %"}
            <input
              type="number"
              step="0.01"
              min="0"
              required
              value={config.value}
              onChange={(e) => setConfig({ ...config, value: number(e.target.value) })}
            />
          </label>
          <label>
            MwSt. in %
            <input
              type="number"
              step="0.1"
              min="0"
              required
              value={config.vat_pct}
              onChange={(e) => setConfig({ ...config, vat_pct: number(e.target.value) })}
            />
          </label>
          <label>
            Rundung
            <select
              value={config.rounding}
              onChange={(e) => setConfig({ ...config, rounding: e.target.value as PricingConfig["rounding"] })}
            >
              {Object.entries(roundingLabels).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </label>
        </div>
        <label className="check">
          <input
            type="checkbox"
            checked={config.cap_at_rrp}
            onChange={(e) => setConfig({ ...config, cap_at_rrp: e.target.checked })}
          />
          Nie über der UVP
        </label>
        <p className="hint">So wird gerechnet: {describe(config)}.</p>
        <div className="decide">
          <button type="submit" className="primary" disabled={busy}>
            {existing ? "Regel ändern" : "Regel anlegen"}
          </button>
        </div>
      </form>
    </>
  );
}
