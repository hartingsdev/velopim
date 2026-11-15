import { useState, type FormEvent } from "react";
import { deleteUsagePolicy, fetchSourceNow, saveUsagePolicy, type FieldGroup, type Policy, type SourceConfig, type UsagePolicyList } from "./api";
import { formatDate } from "./format";
import { SourceEditor } from "./SourceEditor";

interface Props {
  list: UsagePolicyList;
  sources: SourceConfig[];
  onChanged: () => Promise<void>;
  onError: (message: string | null) => void;
}

const groups: { key: FieldGroup; label: string; hint: string }[] = [
  { key: "images", label: "Bilder", hint: "Fotos des Lieferanten" },
  { key: "texts", label: "Texte", hint: "Bezeichnung und Beschreibung" },
  { key: "prices", label: "Preise", hint: "Einkaufspreis und UVP" },
  { key: "attributes", label: "Merkmale", hint: "EAN, Farbe, Rahmenhöhe, weitere Attribute, Verfügbarkeit" },
];

const policyLabels: Record<Policy, string> = {
  use: "verwenden",
  internal_only: "nur intern",
  ignore: "ignorieren",
};

function PolicySelect(props: { value: Policy; onChange: (p: Policy) => void; label: string }) {
  return (
    <select value={props.value} onChange={(e) => props.onChange(e.target.value as Policy)} aria-label={props.label}>
      {Object.entries(policyLabels).map(([value, label]) => (
        <option key={value} value={value}>
          {label}
        </option>
      ))}
    </select>
  );
}

/** What may be used from each supplier's data, per field group and brand. */
export function Sources({ list, sources, onChanged, onError }: Props) {
  async function run(action: () => Promise<void>) {
    try {
      await action();
      onError(null);
    } catch (e) {
      onError((e as Error).message);
    }
    await onChanged();
  }

  return (
    <>
      <p className="hint">
        Hier legst du fest, was aus den Daten eines Lieferanten in deine Produkte und damit in den Shop darf.{" "}
        <strong>Verwenden</strong>: wird ins Produkt übernommen. <strong>Nur intern</strong>: bleibt beim
        Lieferantenartikel sichtbar, kommt aber nie ins Produkt. <strong>Ignorieren</strong>: wie „nur intern“, und
        Änderungen erzeugen keine Aufgaben. Die Einstellung gilt für künftige Übernahmen; bestehende Produkte ändert
        sie nicht.
      </p>
      {list.sources.length === 0 && <p className="empty">Noch kein Lieferant eingerichtet.</p>}
      <details className="task">
        <summary>
          <strong>Neuen Lieferanten anlegen</strong>
        </summary>
        <SourceEditor source={null} onSaved={onChanged} />
      </details>
      {list.sources.map((source) => {
        const own = list.policies.filter((p) => p.source_id === source.id);
        const general = (g: FieldGroup): Policy => own.find((p) => p.brand === "" && p.field_group === g)?.policy ?? "use";
        const exceptions = own.filter((p) => p.brand !== "");
        return (
          <section key={source.id} className="task">
            <div className="task-head">
              <h2>{source.name}</h2>
              <span className="meta">{source.id}</span>
            </div>
            <div className="variants scroll">
              <table>
                <thead>
                  <tr>
                    <th>Daten</th>
                    <th>Alle Marken</th>
                    <th>Ausnahmen je Marke</th>
                  </tr>
                </thead>
                <tbody>
                  {groups.map((g) => (
                    <tr key={g.key}>
                      <td>
                        {g.label}
                        <div className="meta">{g.hint}</div>
                      </td>
                      <td>
                        <PolicySelect
                          value={general(g.key)}
                          label={`${g.label} von ${source.name}, alle Marken`}
                          onChange={(policy) =>
                            void run(() => saveUsagePolicy({ source_id: source.id, brand: "", field_group: g.key, policy }))
                          }
                        />
                      </td>
                      <td>
                        {exceptions
                          .filter((p) => p.field_group === g.key)
                          .map((p) => (
                            <span key={p.brand} className="exception">
                              {p.brand}: {policyLabels[p.policy]}{" "}
                              <button type="button" onClick={() => void run(() => deleteUsagePolicy(p))} aria-label={`Ausnahme für ${p.brand} entfernen`}>
                                ×
                              </button>
                            </span>
                          ))}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <ExceptionForm sourceID={source.id} onSave={(p) => run(() => saveUsagePolicy(p))} />
            <FetchStatus source={sources.find((s) => s.id === source.id)} onChanged={onChanged} onError={onError} />
            <details className="source-settings">
              <summary>Datei-Einstellungen (Spalten, Zeichensatz, Verfügbarkeit)</summary>
              <SourceEditor source={sources.find((s) => s.id === source.id) ?? null} onSaved={onChanged} />
            </details>
          </section>
        );
      })}
    </>
  );
}

/** When the file was last collected, and the button to collect it now. */
function FetchStatus(props: { source: SourceConfig | undefined; onChanged: () => Promise<void>; onError: (m: string | null) => void }) {
  const { source, onChanged, onError } = props;
  const [queued, setQueued] = useState(false);
  if (!source || !source.fetch.url) return null;
  const every = source.fetch.interval_minutes;

  async function fetchNow() {
    try {
      await fetchSourceNow(source!.id);
      setQueued(true);
      onError(null);
      // The job runs in the background; look again shortly.
      window.setTimeout(() => void onChanged().then(() => setQueued(false)), 4000);
    } catch (e) {
      onError((e as Error).message);
    }
  }

  return (
    <div className="fetch-status">
      <p className="item">
        Wird abgeholt von {source.fetch.url},{" "}
        {every === 0 ? "nur auf Knopfdruck" : every % 60 === 0 ? `alle ${every / 60} Stunden` : `alle ${every} Minuten`}.{" "}
        {source.last_fetch_at ? `Zuletzt ${formatDate(source.last_fetch_at)}.` : "Noch nie abgeholt."}
      </p>
      {source.last_fetch_error && <p className="error">Letzter Versuch fehlgeschlagen: {source.last_fetch_error}</p>}
      <div className="decide">
        <button type="button" disabled={queued} onClick={() => void fetchNow()}>
          {queued ? "Wird abgeholt …" : "Jetzt abholen"}
        </button>
      </div>
    </div>
  );
}

function ExceptionForm(props: { sourceID: string; onSave: (p: UsagePolicyList["policies"][number]) => Promise<void> }) {
  const [brand, setBrand] = useState("");
  const [group, setGroup] = useState<FieldGroup>("images");
  const [policy, setPolicy] = useState<Policy>("internal_only");

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (brand.trim() === "") return;
    await props.onSave({ source_id: props.sourceID, brand: brand.trim(), field_group: group, policy });
    setBrand("");
  }

  return (
    <form className="condition exception-form" onSubmit={(e) => void submit(e)}>
      <input type="text" value={brand} onChange={(e) => setBrand(e.target.value)} placeholder="Marke" aria-label="Marke der Ausnahme" />
      <select value={group} onChange={(e) => setGroup(e.target.value as FieldGroup)} aria-label="Daten">
        {groups.map((g) => (
          <option key={g.key} value={g.key}>
            {g.label}
          </option>
        ))}
      </select>
      <PolicySelect value={policy} onChange={setPolicy} label="Richtlinie der Ausnahme" />
      <button type="submit" disabled={brand.trim() === ""}>
        Ausnahme für Marke anlegen
      </button>
    </form>
  );
}
