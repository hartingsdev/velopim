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
  fetch: { url: "", interval_minutes: 0, auth: "" },
  last_fetch_at: null,
  last_fetch_error: null,
};

const intervals = [
  { minutes: 0, label: "nur auf Knopfdruck" },
  { minutes: 15, label: "alle 15 Minuten" },
  { minutes: 60, label: "stündlich" },
  { minutes: 240, label: "alle 4 Stunden" },
  { minutes: 720, label: "alle 12 Stunden" },
  { minutes: 1440, label: "täglich" },
];

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
  const [fetchConfig, setFetchConfig] = useState({ ...emptySource.fetch, ...initial.fetch });
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
        fetch: {
          url: fetchConfig.url.trim(),
          interval_minutes: fetchConfig.url.trim() ? fetchConfig.interval_minutes : 0,
          auth: fetchConfig.url.trim() ? fetchConfig.auth : "",
          user_env: fetchConfig.auth === "basic" ? fetchConfig.user_env?.trim() : undefined,
          secret_env: fetchConfig.auth ? fetchConfig.secret_env?.trim() : undefined,
          header_name: fetchConfig.auth === "header" ? fetchConfig.header_name?.trim() : undefined,
        },
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

      <h3>Automatisch abholen</h3>
      <p className="hint">
        velopim kann die Datei selbst von einer Adresse laden und importieren, statt auf einen Upload zu warten.
        Zugangsdaten stehen nie hier: Du trägst sie in die Datei <code>.env</code> ein und nennst hier nur den Namen der
        Variable. Der Name muss mit <code>VELOPIM_FETCH_</code> beginnen; nach einer Änderung an der <code>.env</code>{" "}
        den Server neu starten.
      </p>
      <div className="fields">
        <label className="wide">
          Adresse der Datei (leer: nicht abholen)
          <input
            type="url"
            value={fetchConfig.url}
            placeholder="https://…"
            onChange={(e) => setFetchConfig({ ...fetchConfig, url: e.target.value })}
          />
        </label>
        <label>
          Wie oft
          <select
            value={fetchConfig.interval_minutes}
            onChange={(e) => setFetchConfig({ ...fetchConfig, interval_minutes: Number(e.target.value) })}
          >
            {intervals.map((i) => (
              <option key={i.minutes} value={i.minutes}>
                {i.label}
              </option>
            ))}
            {!intervals.some((i) => i.minutes === fetchConfig.interval_minutes) && (
              <option value={fetchConfig.interval_minutes}>alle {fetchConfig.interval_minutes} Minuten</option>
            )}
          </select>
        </label>
        <label>
          Anmeldung
          <select
            value={fetchConfig.auth}
            onChange={(e) => setFetchConfig({ ...fetchConfig, auth: e.target.value as typeof fetchConfig.auth })}
          >
            <option value="">keine</option>
            <option value="basic">Benutzername und Passwort</option>
            <option value="bearer">Token (Bearer)</option>
            <option value="header">Schlüssel in einem Kopffeld</option>
          </select>
        </label>
        {fetchConfig.auth === "basic" && (
          <label>
            Variable mit dem Benutzernamen
            <input
              type="text"
              value={fetchConfig.user_env ?? ""}
              placeholder="VELOPIM_FETCH_HARTJE_USER"
              onChange={(e) => setFetchConfig({ ...fetchConfig, user_env: e.target.value.toUpperCase() })}
            />
          </label>
        )}
        {fetchConfig.auth === "header" && (
          <label>
            Name des Kopffelds
            <input
              type="text"
              value={fetchConfig.header_name ?? ""}
              placeholder="X-API-Key"
              onChange={(e) => setFetchConfig({ ...fetchConfig, header_name: e.target.value })}
            />
          </label>
        )}
        {fetchConfig.auth !== "" && (
          <label>
            Variable mit {fetchConfig.auth === "basic" ? "dem Passwort" : "dem Token oder Schlüssel"}
            <input
              type="text"
              value={fetchConfig.secret_env ?? ""}
              placeholder="VELOPIM_FETCH_HARTJE_PASSWORD"
              onChange={(e) => setFetchConfig({ ...fetchConfig, secret_env: e.target.value.toUpperCase() })}
            />
          </label>
        )}
      </div>

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
