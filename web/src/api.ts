// Types mirror internal/api/api.go. They are written by hand; keep both
// sides in sync when the API changes.

export interface TaskAction {
  type: string;
  field: string;
  old: string;
  new: string;
  reason: string;
  rule: string | null;
  /** For a purchase price change: own sales price now and after approval. */
  base_price_cents: number | null;
  proposed_price_cents: number | null;
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
  category: TaskCategory;
  /** Set for tasks about a product of the own catalog. */
  product_id: number | null;
  action: TaskAction | null;
  item: TaskItem | null;
  /** Supplier items of a new-model task; empty otherwise. */
  variants: TaskItem[];
  /** Set when the model already has products: their cut applies. */
  locked_split_by: string[] | null;
}

export type TaskCategory = "price" | "new_model" | "availability" | "removed" | "content" | "import" | "shopify" | "other";

export interface TaskList {
  tasks: Task[];
  open: number;
  closed: number;
  /** Tasks of the requested status and search per category, ignoring the category filter. */
  categories: Partial<Record<TaskCategory, number>>;
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

function queryString(params: Record<string, string | undefined>): string {
  const q = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) if (value) q.set(key, value);
  const s = q.toString();
  return s ? `?${s}` : "";
}

export function listTasks(params: { status?: string; category?: string; q?: string }): Promise<TaskList> {
  return request(`/api/tasks${queryString(params)}`);
}

/** Corrections to a proposal, applied when it is approved. */
export interface Corrections {
  /** Axes that separate the products of a new model. */
  split_by?: string[];
  /** Sales price in cents per supplier article number of a new model. */
  variant_prices?: Record<string, number>;
  /** Sales price in cents instead of the proposed one. */
  price_cents?: number;
}

export function decideTask(id: number, approve: boolean, note: string, corrections: Corrections = {}): Promise<void> {
  return request(`/api/tasks/${id}/decision`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ approve, note, ...(approve ? corrections : {}) }),
  });
}

export interface ImportList {
  runs: ImportRun[];
  sources: { id: string; name: string }[];
}

export function listImportRuns(): Promise<ImportList> {
  return request("/api/import-runs");
}

export interface ImportResult {
  run_id: number;
  status: "done" | "held";
  records: number;
  new: number;
  changed: number;
  removed: number;
  issues: number;
  auto_applied: number;
  pending_approval: number;
}

/** Imports a supplier file for a source. */
export function uploadImport(sourceID: string, file: File): Promise<ImportResult> {
  return request(`/api/sources/${encodeURIComponent(sourceID)}/imports`, {
    method: "POST",
    headers: { "Content-Type": "application/octet-stream" },
    body: file,
  });
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
  /** Products cut from the same model share key and name. */
  model_key: string;
  model_name: string;
  /** What this product was separated by for the shop. */
  split: { axis: string; value: string }[];
  variants: Variant[];
}

export interface ProductList {
  products: Product[];
  total: number;
  unpriced: number;
}

export function listProducts(params: { status?: string; unpriced?: string; q?: string }): Promise<ProductList> {
  return request(`/api/products${queryString(params)}`);
}

export interface SupplierItem {
  id: number;
  source_id: string;
  sku: string;
  ean: string;
  brand: string;
  name: string;
  model_year: number | null;
  color: string;
  frame_size: string;
  cost_cents: number | null;
  rrp_cents: number | null;
  availability: string;
  active: boolean;
  own_sku: string | null;
  product_name: string | null;
}

export async function searchSupplierItems(q: string): Promise<SupplierItem[]> {
  const body = await request<{ items: SupplierItem[] }>(`/api/supplier-items${queryString({ q })}`);
  return body.items;
}

export type RuleActionType =
  | "new_product"
  | "price_change"
  | "availability_status_change"
  | "description_change"
  | "images_change"
  | "deactivation"
  | "attribute_change";

export interface RuleCondition {
  fact: "price_change_pct" | "price_change_abs_pct" | "margin_pct" | "cost_change_cents";
  op: "lt" | "lte" | "gt" | "gte";
  value: number;
}

export interface Rule {
  id: number;
  name: string;
  enabled: boolean;
  action_type: RuleActionType;
  scope: { source_id?: string; brand?: string; category?: string };
  conditions: RuleCondition[];
  outcome: "auto" | "approval";
  guard: boolean;
}

export interface RuleList {
  rules: Rule[];
  sources: { id: string; name: string }[];
}

export function listRules(): Promise<RuleList> {
  return request("/api/rules");
}

export function saveRule(rule: Rule): Promise<Rule> {
  return request(rule.id ? `/api/rules/${rule.id}` : "/api/rules", {
    method: rule.id ? "PUT" : "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(rule),
  });
}

export function deleteRule(id: number): Promise<void> {
  return request(`/api/rules/${id}`, { method: "DELETE" });
}

export interface FieldChange {
  id: number;
  product_id: number;
  variant_sku: string | null;
  field: string;
  old: string | null;
  new: string | null;
  origin: "import" | "rule" | "user" | "system";
  actor: string | null;
  rule: string | null;
  source: string | null;
  reason: string | null;
  created_at: string;
}

export interface Note {
  id: number;
  product_id: number;
  body: string;
  author: string;
  created_at: string;
}

export interface ProductDetail {
  products: (Product & { description_html: string; image_urls: string[]; wanted: boolean })[];
  history: FieldChange[];
  notes: Note[];
}

export function getProduct(id: string): Promise<ProductDetail> {
  return request(`/api/products/${encodeURIComponent(id)}`);
}

function send(method: string, path: string, body: unknown): Promise<void> {
  return request(path, { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
}

export function updateProduct(id: number, change: { name?: string; description_html?: string; wanted?: boolean }): Promise<void> {
  return send("PATCH", `/api/products/${id}`, change);
}

export function setVariantPrice(id: number, priceCents: number): Promise<void> {
  return send("PATCH", `/api/variants/${id}`, { price_cents: priceCents });
}

export function addProductNote(id: number, body: string): Promise<void> {
  return send("POST", `/api/products/${id}/notes`, { body });
}

export type FieldGroup = "images" | "texts" | "prices" | "attributes";
export type Policy = "use" | "internal_only" | "ignore";

export interface UsagePolicy {
  source_id: string;
  brand: string;
  field_group: FieldGroup;
  policy: Policy;
}

export interface UsagePolicyList {
  policies: UsagePolicy[];
  sources: { id: string; name: string }[];
}

export function listUsagePolicies(): Promise<UsagePolicyList> {
  return request("/api/usage-policies");
}

export function saveUsagePolicy(policy: UsagePolicy): Promise<void> {
  return send("PUT", "/api/usage-policies", policy);
}

export function deleteUsagePolicy(p: Pick<UsagePolicy, "source_id" | "brand" | "field_group">): Promise<void> {
  return request(`/api/usage-policies${queryString(p)}`, { method: "DELETE" });
}

export interface SyncOp {
  kind: "create_product" | "update_product" | "set_images" | "set_metafields" | "create_variant" | "update_variant" | "delete_variant";
  sku?: string;
  fields?: string[];
}

export interface PlanProduct {
  product_id: number;
  name: string;
  /** False for a product taken out of the store that is still in Shopify. */
  wanted: boolean;
  in_shopify: boolean;
  last_synced_at: string | null;
  last_error: string | null;
  ops: SyncOp[];
  issues: string[];
  projection: {
    title: string;
    description_html: string;
    vendor: string;
    product_type: string;
    published: boolean;
    options: string[];
    image_urls: string[];
    variants: {
      sku: string;
      barcode: string;
      option_values: string[];
      price_cents: number;
      compare_at_cents: number;
      sellable: boolean;
      delivery_text: string;
    }[];
  };
}

export interface SyncPlan {
  store_configured: boolean;
  write_enabled: boolean;
  products: PlanProduct[];
}

export function getSyncPlan(): Promise<SyncPlan> {
  return request("/api/sync-plan");
}

export interface SyncResult {
  synced: number;
  unchanged: number;
  failed: number;
  products: { product_id: number; name: string; status: "synced" | "unchanged" | "failed"; error?: string; created?: boolean }[];
}

export function runSync(): Promise<SyncResult> {
  return request("/api/sync", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" });
}

/** Accepts a held delivery as complete; the missing articles are deactivated. */
export function confirmImportRun(id: number): Promise<{ run_id: number; removed: number }> {
  return request(`/api/import-runs/${id}/confirm`, { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" });
}
