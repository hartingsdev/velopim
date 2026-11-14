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
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) message += `: ${body.error}`;
    } catch {
      // no JSON body
    }
    throw new ApiError(res.status, message);
  }
  return res.status === 204 ? (undefined as T) : ((await res.json()) as T);
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
  }
}

export function listTasks(status: "open" | "closed"): Promise<TaskList> {
  return request(`/api/tasks?status=${status}`);
}

export function decideTask(id: number, approve: boolean, note: string): Promise<void> {
  return request(`/api/tasks/${id}/decision`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ approve, note }),
  });
}

export async function listImportRuns(): Promise<ImportRun[]> {
  const body = await request<{ runs: ImportRun[] }>("/api/import-runs");
  return body.runs;
}
