import { Plus, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { useOutletContext } from "react-router-dom";
import { TaskTable } from "../components/TaskTable";
import { EmptyState, InlineError, InlineSuccess, PageSkeleton } from "../components/ui";
import { localizeError, useLanguage } from "../i18n";
import type { ConsoleOutletContext } from "../layouts/ConsoleLayout";
import { useAppState } from "../state";
import type { Task } from "../types";

export function TasksPage() {
  const { tasks, runs, loading, runTask, deleteTask } = useAppState();
  const { language, pick } = useLanguage();
  const { openTaskDialog } = useOutletContext<ConsoleOutletContext>();
  const [query, setQuery] = useState("");
  const [runningTask, setRunningTask] = useState<string>();
  const [actionError, setActionError] = useState<string>();
  const [notice, setNotice] = useState<string>();
  const filtered = useMemo(() => tasks.filter((task) => `${task.name} ${task.description}`.toLowerCase().includes(query.toLowerCase())), [query, tasks]);
  const runningTasks = useMemo(() => new Set(runs.filter((run) => run.status === "pending" || run.status === "running").map((run) => run.task_id)), [runs]);

  async function run(task: Task) {
    setRunningTask(task.id);
    setActionError(undefined); setNotice(undefined);
    try {
      await runTask(task.id);
      setNotice(pick(`“${task.name}”已开始运行。`, `“${task.name}” has started.`));
    } catch (caught) {
      setActionError(localizeError(caught, language, pick("无法启动任务，请稍后重试。", "Could not start the task.")));
    } finally { setRunningTask(undefined); }
  }

  async function remove(task: Task) {
    if (!window.confirm(pick(`确定删除“${task.name}”吗？执行历史会继续保留。`, `Delete “${task.name}”? Its run history will be kept.`))) return;
    setActionError(undefined); setNotice(undefined);
    try {
      await deleteTask(task.id);
      setNotice(pick(`“${task.name}”已删除，运行历史仍然保留。`, `“${task.name}” was deleted. Run history is still available.`));
    } catch (caught) {
      setActionError(localizeError(caught, language, pick("无法删除任务，请稍后重试。", "Could not delete the task.")));
    }
  }

  if (loading) return <PageSkeleton />;
  return (
    <section className="panel full-panel">
      <header className="collection-toolbar"><div><h2>{pick("所有任务", "All tasks")}</h2><p>{pick(`${tasks.length} 项任务，${tasks.filter((item) => item.enabled !== false).length} 项启用`, `${tasks.length} tasks, ${tasks.filter((item) => item.enabled !== false).length} active`)}</p></div><div className="toolbar-actions"><label className="search-box"><Search size={15} /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={pick("搜索任务", "Search tasks")} /></label></div></header>
      <div className="collection-feedback"><InlineError message={actionError} /><InlineSuccess message={notice} /></div>
      {filtered.length ? <TaskTable tasks={filtered} runningTask={runningTask} runningTasks={runningTasks} onEdit={openTaskDialog} onRun={run} onDelete={remove} /> : <EmptyState title={query ? pick("没有匹配的任务", "No matching tasks") : pick("还没有任务", "No tasks yet")} body={query ? pick("换一个关键词试试。", "Try another search term.") : pick("创建第一项 AI 定时任务。", "Create your first scheduled AI task.")} action={!query && <button className="button button-primary" onClick={() => openTaskDialog()}><Plus size={15} />{pick("新建任务", "New task")}</button>} />}
    </section>
  );
}
