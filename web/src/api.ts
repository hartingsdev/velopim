// Types mirror internal/api/api.go. They are written by hand; keep both
// sides in sync when the API changes.

export interface TaskAction {
  type: string;
  field: string;
  old: string;
  new: string;
  reason: string;
  rule: string | null;
}

export interface TaskItem {
  source_id: string;
  sku: string;
  brand: string;
  name: string;
  model_year: number | null;
  color: string;
  frame_size: string;
  /** Only filled for the variants of a new-model task. */
  category: string;
  cost_cents: number | null;
  rrp_cents: number | null;
  /** What the matching pricing rule calculates; null without a rule. */
  price_cents: number | null;
  attributes: Record<string, string> | null;
}

export interface Task {
  id: number;
  kind: string;
  title: string;
  detail: string;
  status: "open" | "done" | "dismissed";
  priority: number;
  created_at: string;
  closed_at: string | null;
  closed_by: string | null;
  note: string | null;
  import_run_id: number | null;
  action: TaskAction | null;
  item: TaskItem | null;
  /** Supplier items of a new-model task; empty otherwise. */
  variants: TaskItem[];
  /** Set when the model already has products: their cut applies. */
  locked_split_by: string[] | null;
}

export interface TaskList {
  tasks: Task[];
  open: number;
  closed: number;
}

export interface ImportIssue {
  key?: string;
  field?: string;
  message?: string;
  [extra: string]: unknown;
}

export interface ImportRun {
  id: number;
  source_id: string;
  source_name: string;
  status: "running" | "done" | "failed" | "held";
  record_count: number;
  issue_count: number;
  summary: {
    new?: number;
    changed?: number;
    removed?: number;
    quantity_changes?: number;
    issues?: ImportIssue[] | null;
  };
  error: string | null;
  started_at: string;
  finished_at: string | null;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init);
  if (!res.ok) {
    let message = `Fehler ${res.status}`;
    let issues: string[] = [];
    try {
      const body = (await res.json()) as { error?: string; issues?: string[] };
      if (body.error) message += `: ${body.error}`;
      issues = body.issues ?? [];
    } catch {
      // no JSON body
    }
    throw new ApiError(res.status, message, issues);
  }
  return res.status === 204 ? (undefined as T) : ((await res.json()) as T);
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
    readonly issues: string[] = [],
  ) {
    super(message);
  }
}

export function listTasks(status: "open" | "closed"): Promise<TaskList> {
  return request(`/api/tasks?status=${status}`);
}

export function decideTask(id: number, approve: boolean, note: string, splitBy: string[] = []): Promise<void> {
  return request(`/api/tasks/${id}/decision`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ approve, note, split_by: splitBy }),
  });
}

export async function listImportRuns(): Promise<ImportRun[]> {
  const body = await request<{ runs: ImportRun[] }>("/api/import-runs");
  return body.runs;
}

export interface PricingConfig {
  mode: "markup" | "target_margin";
  value: number;
  vat_pct: number;
  rounding: "none" | "whole_euro" | "end_99" | "end_9";
  cap_at_rrp: boolean;
}

export interface PricingRule {
  id: number;
  brand: string;
  category: string;
  config: PricingConfig;
}

export async function listPricingRules(): Promise<PricingRule[]> {
  const body = await request<{ rules: PricingRule[] }>("/api/pricing-rules");
  return body.rules;
}

export function savePricingRule(brand: string, category: string, config: PricingConfig): Promise<PricingRule> {
  return request("/api/pricing-rules", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ brand, category, config }),
  });
}

export function deletePricingRule(id: number): Promise<void> {
  return request(`/api/pricing-rules/${id}`, { method: "DELETE" });
}

export interface Variant {
  id: number;
  sku: string;
  ean: string;
  option_values: string[];
  price_cents: number | null;
  compare_at_cents: number | null;
  supplier_sku: string | null;
  source_id: string | null;
  cost_cents: number | null;
  rrp_cents: number | null;
  availability: string | null;
}

export interface Product {
  id: number;
  brand: string;
  name: string;
  model_year: number | null;
  category: string;
  status: "draft" | "active" | "archived";
  option_names: string[];
  created_at: string;
  variants: Variant[];
}

export async function listProducts(): Promise<Product[]> {
  const body = await request<{ products: Product[] }>("/api/products");
  return body.products;
}
