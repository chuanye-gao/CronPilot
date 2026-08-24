export type ExecutionStatus = "pending" | "running" | "success" | "failed" | "timeout" | "interrupted";

export interface Delivery {
  type?: string;
  to?: string[];
  on?: string[];
  include_output?: boolean;
}

export interface Task {
  id: string;
  name: string;
  description: string;
  schedule: string;
  timezone: string;
  prompt: string;
  enabled?: boolean;
  timeout: string;
  retry: {
    max_attempts: number;
    delay: string;
  };
  delivery?: Delivery;
  created_at: string;
  updated_at: string;
  next_run?: string;
}

export type TaskInput = Omit<Task, "id" | "created_at" | "updated_at" | "next_run">;

export interface Execution {
  id: string;
  task_id: string;
  task_name: string;
  status: ExecutionStatus;
  started_at: string;
  finished_at?: string;
  output?: string;
  error?: string;
  attempts: number;
  delivery_status?: "pending" | "sent" | "failed";
  delivery_error?: string;
}

export interface Health {
  status: string;
  model: string;
  provider_configured: boolean;
  fallback_model: string;
  fallback_configured: boolean;
  timezone: string;
  email_configured: boolean;
  storage: string;
  web_search_configured: boolean;
  web_search_status: "healthy" | "unavailable" | "disabled";
  web_search_provider: string;
  relay_configured: boolean;
}

export type IntegrationName = "database" | "deepseek" | "gemini" | "tavily" | "email";

export interface IntegrationTest {
  integration: IntegrationName;
  status: "healthy" | "failed";
  duration_ms: number;
}

export interface EmailStatus {
  configured: boolean;
  provider: string;
}

export interface User {
  id: string;
  name: string;
  email: string;
  email_verified: boolean;
  created_at: string;
  updated_at: string;
}

export interface AssistantMessage {
  role: "user" | "assistant";
  content: string;
}

export interface TaskAssistantDraft {
  name: string;
  description: string;
  schedule: string;
  schedule_label: string;
  timezone: string;
  prompt: string;
  notify_email: boolean;
  email_events: "all" | "failures" | "success";
  recipient: string;
}

export interface TaskAssistantPlan {
  reply: string;
  ready: boolean;
  missing_fields: string[];
  quick_replies: string[];
  draft: TaskAssistantDraft;
}
