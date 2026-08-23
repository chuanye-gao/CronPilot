import { Check, CircleDashed, ExternalLink } from "lucide-react";
import { PageSkeleton } from "../components/ui";
import { useLanguage } from "../i18n";
import { useAppState } from "../state";

export function SystemPage() {
  const { health, email, loading } = useAppState();
  const { pick } = useLanguage();
  if (loading) return <PageSkeleton />;
  const checks = [
    { label: pick("网站服务", "Web service"), value: health?.status === "ok" ? pick("正常", "Healthy") : pick("不可用", "Unavailable"), ok: health?.status === "ok" },
    { label: pick("任务调度", "Scheduler"), value: health?.status === "ok" ? pick("运行中", "Running") : pick("检查中", "Checking"), ok: health?.status === "ok" },
    { label: pick("数据库", "Database"), value: health?.storage || pick("未知", "Unknown"), ok: health?.storage === "sqlite" },
    { label: pick("AI 模型", "AI model"), value: health?.provider_configured ? health.model : pick("未配置", "Not configured"), ok: Boolean(health?.provider_configured) },
    { label: pick("联网检索", "Web research"), value: health?.web_search_status === "healthy" ? health.web_search_provider : health?.web_search_status === "unavailable" ? pick("暂时不可用", "Temporarily unavailable") : pick("未启用", "Disabled"), ok: health?.web_search_status === "healthy" },
    { label: pick("邮件服务", "Email"), value: email?.configured ? pick("已连接", "Connected") : pick("未配置", "Not configured"), ok: Boolean(email?.configured) },
  ];
  return (
    <div className="system-page">
      <section className="system-health panel">
        <header className="panel-header"><div><h2>{pick("运行状态", "Runtime status")}</h2></div><span className={`overall-health ${health?.status === "ok" ? "healthy" : ""}`}><i />{health?.status === "ok" ? pick("系统运行正常", "All systems operational") : pick("系统需要检查", "System needs attention")}</span></header>
        <div className="health-list">{checks.map((check) => <div key={check.label}><span className={`check-icon ${check.ok ? "ok" : "waiting"}`}>{check.ok ? <Check size={14} /> : <CircleDashed size={14} />}</span><strong>{check.label}</strong><code>{check.value}</code></div>)}</div>
      </section>
      <section className="build-info"><div><small>{pick("部署方式", "Deployment")}</small><strong>Docker</strong></div><div><small>{pick("数据库", "Database")}</small><strong>SQLite</strong></div><div><small>{pick("默认时区", "Default time zone")}</small><strong>{health?.timezone || "Asia/Shanghai"}</strong></div><a href="https://github.com/chuanye-gao/CronPilot" target="_blank" rel="noreferrer">GitHub <ExternalLink size={14} /></a></section>
    </div>
  );
}
