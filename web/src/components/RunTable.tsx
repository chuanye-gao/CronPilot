import { ChevronRight } from "lucide-react";
import { useLanguage } from "../i18n";
import type { Execution } from "../types";
import { executionDuration, formatDate } from "../utils";
import { LinkifiedOutput, StatusBadge } from "./ui";

export function RunTable({ runs, onSelect, compact }: { runs: Execution[]; onSelect: (run: Execution) => void; compact?: boolean }) {
  const { language, pick } = useLanguage();
  return (
    <div className={`run-table ${compact ? "compact" : ""}`}>
      {!compact && <div className="run-table-head"><span>{pick("任务", "Task")}</span><span>{pick("状态", "Status")}</span><span>{pick("尝试", "Attempts")}</span><span>{pick("耗时", "Duration")}</span><span>{pick("开始时间", "Started")}</span><span /></div>}
      {runs.map((run) => (
        <button className="run-item" key={run.id} onClick={() => onSelect(run)}>
          <span className="run-name"><strong>{run.task_name}</strong><code>{run.id}</code></span>
          <StatusBadge status={run.status} />
          {!compact && <span className="run-cell">{run.attempts || 0}</span>}
          <span className="run-cell">{executionDuration(run, language)}</span>
          <time>{formatDate(run.started_at, language)}</time>
          <ChevronRight size={15} />
        </button>
      ))}
    </div>
  );
}

export function RunDetail({ run, onClose }: { run?: Execution; onClose: () => void }) {
  const { language, pick } = useLanguage();
  if (!run) return null;
  return (
    <div className="drawer-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <aside className="run-drawer">
        <header><div><h2>{run.task_name}</h2></div><button className="icon-button" aria-label={pick("关闭", "Close")} onClick={onClose}>×</button></header>
        <div className="run-summary"><StatusBadge status={run.status} /><span>{run.attempts || 0} {pick("次尝试", "attempts")}</span><span>{executionDuration(run, language)}</span><time>{formatDate(run.started_at, language)}</time></div>
        <dl className="detail-list"><div><dt>{pick("运行 ID", "Run ID")}</dt><dd>{run.id}</dd></div><div><dt>{pick("任务 ID", "Task ID")}</dt><dd>{run.task_id}</dd></div><div><dt>{pick("邮件状态", "Email status")}</dt><dd>{run.delivery_status || pick("未配置", "Not configured")}</dd></div></dl>
        <div className="output-heading">{run.output ? pick("任务输出", "Output") : run.error ? pick("错误信息", "Error") : pick("当前状态", "Status")}</div>
        <LinkifiedOutput className="run-output" value={run.output || run.error || (run.status === "running" ? pick("任务正在运行……", "Task is running…") : pick("等待任务开始。", "Waiting to start."))} />
        {run.delivery_error && <div className="inline-error">{pick("邮件发送失败：", "Email delivery failed: ")}{run.delivery_error}</div>}
      </aside>
    </div>
  );
}
