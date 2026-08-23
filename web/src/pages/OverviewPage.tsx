import { ArrowRight, CheckCircle2, Clock3, Plus, Sparkles, TrendingUp } from "lucide-react";
import { useMemo, useState } from "react";
import { Link, useOutletContext } from "react-router-dom";
import { RunDetail, RunTable } from "../components/RunTable";
import { TaskTable } from "../components/TaskTable";
import { EmptyState, InlineError, InlineSuccess, PageSkeleton } from "../components/ui";
import { localizeError, useLanguage } from "../i18n";
import type { ConsoleOutletContext } from "../layouts/ConsoleLayout";
import { useAppState } from "../state";
import type { Execution, Task } from "../types";
import { formatTime, relativeTime } from "../utils";

export function OverviewPage() {
  const { tasks, runs, loading, runTask } = useAppState();
  const { language, pick } = useLanguage();
  const { openTaskDialog } = useOutletContext<ConsoleOutletContext>();
  const [selectedRun, setSelectedRun] = useState<Execution>();
  const [runningTask, setRunningTask] = useState<string>();
  const [actionError, setActionError] = useState<string>();
  const [notice, setNotice] = useState<string>();
  const runningTasks = useMemo(() => new Set(runs.filter((run) => run.status === "pending" || run.status === "running").map((run) => run.task_id)), [runs]);
  const activeTasks = tasks.filter((task) => task.enabled !== false);
  const completed = runs.filter((run) => ["success", "failed", "timeout", "interrupted"].includes(run.status));
  const successful = completed.filter((run) => run.status === "success").length;
  const successRate = completed.length ? Math.round(successful / completed.length * 100) : undefined;
  const nextTask = activeTasks.filter((task) => task.next_run).sort((a, b) => new Date(a.next_run!).getTime() - new Date(b.next_run!).getTime())[0];

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

  if (loading) return <PageSkeleton />;

  return (
    <>
      <InlineError message={actionError} />
      <InlineSuccess message={notice} />
      <section className="welcome-row"><div><h2>{pick("你的任务运行情况", "Your task activity")}</h2><p>{pick("查看计划、运行状态和最近结果。", "Review schedules, run status, and recent results.")}</p></div><span className="today-card"><Clock3 size={18} /><span><strong>{new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { weekday: "long" }).format(new Date())}</strong><small>{new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { year: "numeric", month: "long", day: "numeric" }).format(new Date())}</small></span></span></section>
      <section className="metric-grid">
        <article><span className="metric-icon lime"><Sparkles /></span><div><small>{pick("启用任务", "Active tasks")}</small><strong>{String(activeTasks.length).padStart(2, "0")}</strong><p>{pick(`${tasks.length - activeTasks.length} 个已暂停`, `${tasks.length - activeTasks.length} paused`)}</p></div></article>
        <article><span className="metric-icon violet"><TrendingUp /></span><div><small>{pick("成功率", "Success rate")}</small><strong>{successRate === undefined ? "—" : `${successRate}%`}</strong><p>{completed.length ? pick(`${successful} / ${completed.length} 次成功`, `${successful} of ${completed.length} succeeded`) : pick("暂无完成记录", "No completed runs")}</p></div></article>
        <article><span className="metric-icon blue"><CheckCircle2 /></span><div><small>{pick("运行记录", "Runs")}</small><strong>{String(runs.length).padStart(2, "0")}</strong><p>{runs.some((item) => item.status === "running") ? pick("当前有任务运行", "A task is running") : pick("当前没有任务运行", "No task is running")}</p></div></article>
        <article className="next-metric"><span className="metric-icon dark"><Clock3 /></span><div><small>{pick("下次运行", "Next run")}</small><strong>{formatTime(nextTask?.next_run, language)}</strong><p>{nextTask ? `${nextTask.name} · ${relativeTime(nextTask.next_run, language)}` : pick("暂无计划", "Nothing scheduled")}</p></div></article>
      </section>

      <section className="dashboard-grid">
        <div className="panel upcoming-panel">
          <header className="panel-header"><div><h2>{pick("任务计划", "Scheduled tasks")}</h2></div><Link to="/app/tasks">{pick("查看全部", "View all")} <ArrowRight size={14} /></Link></header>
          {tasks.length ? <TaskTable tasks={tasks.slice(0, 4)} compact runningTask={runningTask} runningTasks={runningTasks} onEdit={openTaskDialog} onRun={run} /> : <EmptyState title={pick("创建第一项任务", "Create your first task")} body={pick("设置时间和指令，CronPilot 会按计划运行。", "Set a schedule and instructions. CronPilot will run it on time.")} action={<button className="button button-primary" onClick={() => openTaskDialog()}><Plus size={15} />{pick("新建任务", "New task")}</button>} />}
        </div>
        <div className="panel activity-panel">
          <header className="panel-header"><div><h2>{pick("最近运行", "Recent runs")}</h2></div><Link to="/app/runs">{pick("查看全部", "View all")} <ArrowRight size={14} /></Link></header>
          {runs.length ? <RunTable runs={runs.slice(0, 5)} compact onSelect={setSelectedRun} /> : <EmptyState title={pick("暂无运行记录", "No runs yet")} body={pick("手动运行任务，或等待下一次计划。", "Run a task now or wait for its schedule.")} />}
        </div>
      </section>
      <RunDetail run={selectedRun} onClose={() => setSelectedRun(undefined)} />
    </>
  );
}
