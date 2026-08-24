import type { AssistantMessage, EmailStatus, Execution, Health, IntegrationName, IntegrationTest, Task, TaskAssistantDraft, TaskAssistantPlan, TaskInput, User } from "./types";

export class APIError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

export interface TaskDraftTestJob {
  id: string;
  status: "running" | "success" | "failed";
  output?: string;
  error?: string;
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...options,
    credentials: "same-origin",
    headers: options?.body ? { "Content-Type": "application/json", ...options.headers } : options?.headers,
  });
  if (response.status === 204) return undefined as T;
  const text = await response.text();
  let body: { error?: string; error_id?: string } | undefined;
  if (text) {
    try {
      body = JSON.parse(text) as { error?: string; error_id?: string };
    } catch {
      body = { error: text.trim() || `Request failed (${response.status})` };
    }
  }
  if (!response.ok) {
    const suffix = body?.error_id ? ` (${body.error_id})` : "";
    if (response.status === 401 && path !== "/api/auth/login" && path !== "/api/auth/me") {
      window.dispatchEvent(new Event("cronpilot:unauthorized"));
    }
    throw new APIError(response.status, `${body?.error || `Request failed (${response.status})`}${suffix}`);
  }
  return body as T;
}

export const api = {
  me: () => request<{ user: User }>("/api/auth/me"),
  register: (value: { name: string; email: string; password: string }) => request<{ user: User; verification_required: boolean }>("/api/auth/register", { method: "POST", body: JSON.stringify(value) }),
  login: (value: { email: string; password: string }) => request<{ user: User }>("/api/auth/login", { method: "POST", body: JSON.stringify(value) }),
  verifyEmail: (token: string) => request<{ user: User; verified: boolean }>("/api/auth/verify", { method: "POST", body: JSON.stringify({ token }) }),
  logout: () => request<void>("/api/auth/logout", { method: "POST" }),
  health: () => request<Health>("/api/health"),
  tasks: () => request<Task[]>("/api/tasks"),
  createTask: (value: TaskInput) => request<Task>("/api/tasks", { method: "POST", body: JSON.stringify(value) }),
  updateTask: (id: string, value: TaskInput) => request<Task>(`/api/tasks/${id}`, { method: "PUT", body: JSON.stringify(value) }),
  deleteTask: (id: string) => request<void>(`/api/tasks/${id}`, { method: "DELETE" }),
  runTask: (id: string) => request<Execution>(`/api/tasks/${id}/run`, { method: "POST" }),
  runs: () => request<Execution[]>("/api/executions?limit=100"),
  run: (id: string) => request<Execution>(`/api/executions/${id}`),
  emailStatus: () => request<EmailStatus>("/api/email/status"),
  testEmail: (to: string) => request<{ status: string; to: string }>("/api/email/test", { method: "POST", body: JSON.stringify({ to }) }),
  testIntegration: (name: IntegrationName) => request<IntegrationTest>(`/api/integrations/${name}/test`, { method: "POST" }),
  planTask: (language: "zh" | "en", messages: AssistantMessage[], draft: TaskAssistantDraft) => request<TaskAssistantPlan>("/api/task-assistant/plan", { method: "POST", body: JSON.stringify({ language, messages, draft }) }),
  testTaskDraft: (draft: TaskAssistantDraft) => request<TaskDraftTestJob>("/api/task-assistant/test", { method: "POST", body: JSON.stringify(draft) }),
  taskDraftTest: (id: string) => request<TaskDraftTestJob>(`/api/task-assistant/test/${encodeURIComponent(id)}`),
};
