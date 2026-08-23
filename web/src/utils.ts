import type { Execution, ExecutionStatus } from "./types";
import type { Language } from "./i18n";

const locale = (language: Language) => language === "zh" ? "zh-CN" : "en-US";

export function formatDate(value: string | undefined, language: Language) {
  if (!value) return "—";
  return new Intl.DateTimeFormat(locale(language), { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value));
}

export function formatTime(value: string | undefined, language: Language) {
  if (!value) return "—";
  return new Intl.DateTimeFormat(locale(language), { hour: "2-digit", minute: "2-digit" }).format(new Date(value));
}

export function relativeTime(value: string | undefined, language: Language) {
  if (!value) return language === "zh" ? "未安排" : "Not scheduled";
  const distance = new Date(value).getTime() - Date.now();
  if (distance <= 0) return language === "zh" ? "即将运行" : "Starting soon";
  const minutes = Math.round(distance / 60_000);
  if (minutes < 60) return language === "zh" ? `${minutes} 分钟后` : `in ${minutes} min`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return language === "zh" ? `${hours} 小时后` : `in ${hours} hr`;
  const days = Math.round(hours / 24);
  return language === "zh" ? `${days} 天后` : `in ${days} days`;
}

export function executionDuration(run: Execution, language: Language) {
  if (!run.finished_at) return run.status === "running" ? (language === "zh" ? "运行中" : "Running") : "—";
  const milliseconds = new Date(run.finished_at).getTime() - new Date(run.started_at).getTime();
  if (milliseconds < 1_000) return `${milliseconds}ms`;
  if (milliseconds < 60_000) return `${(milliseconds / 1_000).toFixed(1)}s`;
  return `${(milliseconds / 60_000).toFixed(1)}m`;
}

const statusLabels: Record<ExecutionStatus, [string, string]> = {
  pending: ["等待中", "Pending"], running: ["运行中", "Running"], success: ["成功", "Success"],
  failed: ["失败", "Failed"], timeout: ["超时", "Timeout"], interrupted: ["已中断", "Interrupted"],
};

export function statusText(status: ExecutionStatus, language: Language) {
  return statusLabels[status][language === "zh" ? 0 : 1];
}

export function cronSummary(schedule: string, language: Language) {
  const parts = schedule.trim().split(/\s+/);
  if (parts.length !== 5) return schedule;
  const [minute, hour, , , weekday] = parts;
  if (weekday === "1-5") return `${language === "zh" ? "工作日" : "Weekdays"} ${hour.padStart(2, "0")}:${minute.padStart(2, "0")}`;
  if (weekday !== "*") return `${language === "zh" ? "每周" : "Weekly"} ${weekday} · ${hour.padStart(2, "0")}:${minute.padStart(2, "0")}`;
  if (hour !== "*" && minute !== "*") return `${language === "zh" ? "每天" : "Daily"} ${hour.padStart(2, "0")}:${minute.padStart(2, "0")}`;
  return schedule;
}
