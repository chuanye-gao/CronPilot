import { CalendarClock, CirclePlay, MoreHorizontal, Pause, Pencil, Trash2 } from "lucide-react";
import { useLanguage } from "../i18n";
import type { Task } from "../types";
import { cronSummary, formatTime, relativeTime } from "../utils";

interface Props {
  tasks: Task[];
  compact?: boolean;
  runningTask?: string;
  runningTasks?: ReadonlySet<string>;
  onEdit: (task: Task) => void;
  onRun: (task: Task) => void;
  onDelete?: (task: Task) => void;
}

export function TaskTable({ tasks, compact, runningTask, runningTasks, onEdit, onRun, onDelete }: Props) {
  const { language, pick } = useLanguage();
  return (
    <div className="task-table">
      {tasks.map((task) => {
        const isRunning = runningTask === task.id || runningTasks?.has(task.id);
        return (
        <article className="task-item" key={task.id}>
          <span className={`task-state ${task.enabled === false ? "paused" : "active"}`}><CalendarClock size={17} /></span>
          <div className="task-identity">
            <div><h3>{task.name}</h3><span className={`state-label ${task.enabled === false ? "paused" : "active"}`}>{task.enabled === false ? pick("已暂停", "Paused") : pick("已启用", "Active")}</span></div>
            <p>{task.description || pick("没有说明", "No description")}</p>
          </div>
          <div className="task-schedule"><strong>{cronSummary(task.schedule, language)}</strong><span>{task.timezone}</span></div>
          <div className="task-next"><strong>{task.enabled === false ? "—" : formatTime(task.next_run, language)}</strong><span>{task.enabled === false ? pick("不会自动运行", "Automatic runs off") : relativeTime(task.next_run, language)}</span></div>
          <div className="task-actions">
            <button className="icon-button run-action" disabled={isRunning} onClick={() => onRun(task)} aria-label={`${isRunning ? pick("正在运行", "Running") : pick("立即运行", "Run now")} ${task.name}`} title={isRunning ? pick("任务正在运行", "Task is running") : pick("立即运行", "Run now")}><CirclePlay size={17} /></button>
            <button className="icon-button" onClick={() => onEdit(task)} aria-label={`${pick("编辑", "Edit")} ${task.name}`} title={pick("编辑", "Edit")}><Pencil size={16} /></button>
            {!compact && onDelete ? <button className="icon-button danger-action" onClick={() => onDelete(task)} aria-label={`${pick("删除", "Delete")} ${task.name}`} title={pick("删除", "Delete")}><Trash2 size={16} /></button> : <MoreHorizontal className="task-more" size={17} />}
          </div>
        </article>
        );
      })}
      {tasks.length === 0 && <div className="table-empty"><Pause size={20} /><span>{pick("还没有任务", "No tasks yet")}</span></div>}
    </div>
  );
}
