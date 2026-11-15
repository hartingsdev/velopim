import { useState, type FormEvent } from "react";
import { previewCsv, saveSource, type CsvPreview, type SourceConfig } from "./api";
import { AvailabilityRules, type MappingRule } from "./AvailabilityRules";

const targets: { key: string; label: string; required?: boolean }[] = [
  { key: "sku", label: "Artikelnummer", required: true },
  { key: "ean", label: "EAN" },
  { key: "mpn", label: "Herstellernummer" },
  { key: "brand", label: "Marke" },
  { key: "name", label: "Bezeichnung" },
  { key: "model_year", label: "Modelljahr" },
  { key: "category", label: "Kategorie" },
  { key: "color", label: "Farbe" },
  { key: "frame_size", label: "Rahmenhöhe" },
  { key: "description", label: "Beschreibung" },
  { key: "images", label: "Bilder" },
  { key: "cost", label: "Einkaufspreis" },
  { key: "rrp", label: "UVP" },
];

const emptySource: SourceConfig = {
  id: "",
  name: "",
  connector_kind: "csvfile",
  connector_config: { delimiter: ";", decimal_comma: true, columns: {}, encoding: "" },
  availability_mapping: [],
};

/** Form to create a supplier or change how its CSV files are read. */
export function SourceEditor(props: { source: SourceConfig | null; onSaved: () => Promise<void> }) {
  const initial = props.source ?? emptySource;
  const isNew = props.source === null;
  const [id, setId] = useState(initial.id);
  const [name, setName] = useState(initial.name);
  const [config, setConfig] = useState(initial.connector_config);
  const [attributes, setAttributes] = useState((initial.connector_config.attribute_columns ?? []).join(", "));
  const [rules, setRules] = useState<MappingRule[]>(() =>
    (initial.availability_mapping as MappingRule[]).map((r) => ({ ...r, when: r.when ?? [] })),
  );
  const [preview, setPreview] = useState<CsvPreview | null>(null);
  const [problem, setProblem] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);

  async function readFile(file: File | undefined) {
    if (!file) return;
    setProblem(null);
    try {
      const p = await previewCsv(file);
      setPreview(p);
      // Take over what the file tells, keep what was set by hand.
      setConfig((c) => ({ ...c, delimiter: p.delimiter, encoding: c.encoding || "" }));
    } catch (e) {
      setProblem((e as Error).message);
    }
  }

  async function save(e: FormEvent) {
    e.preventDefault();
    setProblem(null);
    setSaved(false);
    const incomplete = rules.findIndex((r) => r.when.some((c) => c.field.trim() === ""));
    if (incomplete >= 0) {
      setProblem(`Verfügbarkeit, Regel ${incomplete + 1}: Bei einer Bedingung fehlt die Spalte.`);
      return;
    }
    const columns = Object.fromEntries(Object.entries(config.columns).filter(([, column]) => column.trim() !== ""));
    setBusy(true);
    try {
      await saveSource(id.trim(), {
        name: name.trim(),
        connector_config: {
          ...config,
          columns,
          attribute_columns: attributes.split(",").map((a) => a.trim()).filter(Boolean),
        },
        availability_mapping: rules,
      });
      setSaved(true);
      await props.onSaved();
    } catch (err) {
      setProblem((err as Error).message);
    }
    setBusy(false);
  }

  const columnList = `columns-${id || "new"}`;

  return (
    <form className="rule-form source-form" onSubmit={(e) => void save(e)}>
      <div className="fields">
        <label>
          Name
          <input type="text" required value={name} onChange={(e) => setName(e.target.value)} placeholder="z. B. Hartje" />
        </label>
        <label>
          Kürzel
          <input
            type="text"
            required
            disabled={!isNew}
            value={id}
            onChange={(e) => setId(e.target.value.toLowerCase())}
            placeholder="z. B. hartje"
            pattern="[a-z0-9][a-z0-9-]*"
            title="Kleinbuchstaben, Ziffern und Bindestriche"
          />
        </label>
        <label>
          Beispieldatei (zum Erkennen der Spalten)
          <input type="file" onChange={(e) => void readFile(e.target.files?.[0])} />
        </label>
      </div>

      {preview && (
        <div className="variants scroll">
          <p className="hint">
            Erkannt: Trennzeichen „{preview.delimiter === "\t" ? "Tab" : preview.delimiter}“, Zeichensatz{" "}
            {preview.encoding === "windows-1252" ? "Windows-1252" : "UTF-8"}, {preview.columns.length} Spalten. Die Datei
            wurde nur gelesen, nicht importiert.
          </p>
          <table>
            <thead>
              <tr>
                {preview.columns.map((c) => (
                  <th key={c}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {preview.rows.map((row, i) => (
                <tr key={i}>
                  {row.map((cell, j) => (
                    <td key={j}>{cell}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="fields">
        <label>
          Trennzeichen
          <input
            type="text"
            required
            maxLength={1}
            value={config.delimiter}
            onChange={(e) => setConfig({ ...config, delimiter: e.target.value })}
          />
        </label>
        <label>
          Zeichensatz
          <select value={config.encoding ?? ""} onChange={(e) => setConfig({ ...config, encoding: e.target.value })}>
            <option value="">automatisch erkennen</option>
            <option value="utf-8">UTF-8</option>
            <option value="windows-1252">Windows-1252</option>
          </select>
        </label>
        <label>
          Trennzeichen für mehrere Bilder
          <input
            type="text"
            value={config.image_separator ?? ""}
            placeholder="|"
            onChange={(e) => setConfig({ ...config, image_separator: e.target.value })}
          />
        </label>
      </div>
      <label className="check">
        <input
          type="checkbox"
          checked={config.decimal_comma}
          onChange={(e) => setConfig({ ...config, decimal_comma: e.target.checked })}
        />
        Preise mit Komma als Dezimalzeichen (1.299,00)
      </label>

      <h3>Welche Spalte enthält was?</h3>
      <datalist id={columnList}>
        {(preview?.columns ?? []).map((c) => (
          <option key={c} value={c} />
        ))}
      </datalist>
      <div className="fields">
        {targets.map((t) => (
          <label key={t.key}>
            {t.label}
            {t.required && " (Pflicht)"}
            <input
              type="text"
              list={columnList}
              required={t.required}
              value={config.columns[t.key] ?? ""}
              placeholder="Spaltenname"
              onChange={(e) => setConfig({ ...config, columns: { ...config.columns, [t.key]: e.target.value } })}
            />
          </label>
        ))}
        <label className="wide">
          Weitere Spalten als Merkmale übernehmen (mit Komma getrennt, z. B. Antrieb, Rahmenform)
          <input type="text" value={attributes} onChange={(e) => setAttributes(e.target.value)} />
        </label>
      </div>

      <h3>Verfügbarkeit</h3>
      <p className="hint">
        Die Regeln werden von oben nach unten geprüft, die erste passende gilt. Passt keine, ist die Verfügbarkeit
        unbekannt und das Rad nicht verkaufbar. Eine letzte Regel ohne Bedingung fängt alle übrigen Artikel auf.
      </p>
      <AvailabilityRules rules={rules} onChange={setRules} columnList={columnList} />

      {problem && (
        <p className="error" role="alert">
          {problem}
        </p>
      )}
      {saved && (
        <p className="hint" role="status">
          Gespeichert. Die Einstellung gilt ab dem nächsten Import.
        </p>
      )}
      <div className="decide">
        <button type="submit" className="primary" disabled={busy}>
          {isNew ? "Lieferant anlegen" : "Einstellungen speichern"}
        </button>
      </div>
    </form>
  );
}
