import { RefreshCw } from "lucide-react";
import { useState } from "react";
import { RunDetail, RunTable } from "../components/RunTable";
import { EmptyState, PageSkeleton } from "../components/ui";
import { useLanguage } from "../i18n";
import { useAppState } from "../state";
import type { Execution } from "../types";

export function RunsPage() {
  const { runs, loading, refresh } = useAppState();
  const { pick } = useLanguage();
  const [selected, setSelected] = useState<Execution>();
  if (loading) return <PageSkeleton />;
  return (
    <section className="panel full-panel">
      <header className="collection-toolbar"><div><h2>{pick("全部运行记录", "All runs")}</h2><p>{pick("查看每次运行的状态、耗时和结果。", "Review the status, duration, and result of every run.")}</p></div><button className="button button-ghost" onClick={() => void refresh()}><RefreshCw size={15} />{pick("刷新", "Refresh")}</button></header>
      {runs.length ? <RunTable runs={runs} onSelect={setSelected} /> : <EmptyState title={pick("暂无运行记录", "No runs yet")} body={pick("运行任务后，结果会显示在这里。", "Task results will appear here after a run.")} />}
      <RunDetail run={selected} onClose={() => setSelected(undefined)} />
    </section>
  );
}
