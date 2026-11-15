// German labels and value formatting for what the API returns.

const euro = new Intl.NumberFormat("de-DE", { style: "currency", currency: "EUR" });
const dateTime = new Intl.DateTimeFormat("de-DE", { dateStyle: "medium", timeStyle: "short" });

export function formatDate(iso: string | null): string {
  return iso ? dateTime.format(new Date(iso)) : "";
}

export function formatCents(cents: number): string {
  return euro.format(cents / 100);
}

const fieldLabels: Record<string, string> = {
  cost_cents: "Einkaufspreis",
  rrp_cents: "UVP",
  "availability.status": "Verfügbarkeit",
  name: "Bezeichnung",
  description: "Beschreibung",
  image_urls: "Bilder",
  ean: "EAN",
  mpn: "Herstellernummer",
  brand: "Marke",
  category: "Kategorie",
  color: "Farbe",
  frame_size: "Rahmenhöhe",
  model_year: "Modelljahr",
};

export function fieldLabel(field: string): string {
  if (field.startsWith("attributes.")) return field.slice("attributes.".length);
  return fieldLabels[field] ?? field;
}

const availabilityLabels: Record<string, string> = {
  in_stock: "Auf Lager",
  orderable: "Bestellbar",
  preorder: "Vorbestellbar",
  on_request: "Auf Anfrage",
  unavailable: "Nicht verfügbar",
  discontinued: "Ausgelaufen",
  unknown: "Unbekannt",
};

export function formatValue(field: string, value: string): string {
  if (field.endsWith("_cents")) {
    const cents = Number(value);
    // 0 means the supplier sent no price.
    return value === "" || cents === 0 ? "kein Preis" : formatCents(cents);
  }
  if (field === "availability.status") return availabilityLabels[value] ?? value;
  if (field === "model_year" && value === "0") return "leer";
  return value === "" ? "leer" : value;
}

/** Difference between two cent values as text, e.g. "−50,00 € (−5,0 %)". */
export function priceDelta(oldValue: string, newValue: string): string | null {
  const a = Number(oldValue);
  const b = Number(newValue);
  if (!a || !b) return null;
  const sign = b > a ? "+" : "−";
  const pct = (Math.abs(b - a) / a) * 100;
  return `${sign}${formatCents(Math.abs(b - a))} (${sign}${pct.toFixed(1).replace(".", ",")} %)`;
}

const reasonLabels: Record<string, string> = {
  "guard rule matched": "Schutzregel verlangt Freigabe",
  "most specific rule matched": "Regel verlangt Freigabe",
  "equally specific rules disagree, approval wins": "Regeln widersprechen sich, daher Freigabe",
  "no rule matched, approval by default": "Keine Regel passt, daher Freigabe",
};

export function reasonLabel(reason: string): string {
  return reasonLabels[reason] ?? reason;
}

const runStatusLabels: Record<string, string> = {
  running: "Läuft",
  done: "Abgeschlossen",
  failed: "Fehlgeschlagen",
  held: "Angehalten",
};

export function runStatusLabel(status: string): string {
  return runStatusLabels[status] ?? status;
}

const priorityLabels: Record<number, string> = { 1: "Hoch", 2: "Normal", 3: "Niedrig" };

export function priorityLabel(priority: number): string {
  return priorityLabels[priority] ?? String(priority);
}

export function axisLabel(axis: string): string {
  if (axis === "color") return "Farbe";
  if (axis === "frame_size") return "Rahmenhöhe";
  return axis.startsWith("attributes.") ? axis.slice("attributes.".length) : axis;
}

export function availabilityLabel(status: string | null): string {
  return status ? (availabilityLabels[status] ?? status) : "–";
}

export function formatPrice(cents: number | null): string {
  return cents === null ? "–" : formatCents(cents);
}

const productStatusLabels: Record<string, string> = { draft: "Entwurf", active: "Aktiv", archived: "Archiviert" };

export function productStatusLabel(status: string): string {
  return productStatusLabels[status] ?? status;
}

/** Amount for a text field, e.g. "1.899,00". */
export function formatAmount(cents: number | null): string {
  return cents === null ? "" : (cents / 100).toLocaleString("de-DE", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

/** Parses "1.899,00", "1899,5" or "1899" into cents; null when it is not an amount. */
export function parseAmount(text: string): number | null {
  const cleaned = text.trim().replace(/\s|€/g, "").replace(/\./g, "").replace(",", ".");
  if (!/^\d+(\.\d{1,2})?$/.test(cleaned)) return null;
  return Math.round(Number(cleaned) * 100);
}
