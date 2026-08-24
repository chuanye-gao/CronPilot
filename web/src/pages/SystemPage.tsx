import { Check, CircleDashed, ExternalLink, LoaderCircle, Play } from "lucide-react";
import { useState } from "react";
import { api } from "../api";
import { PageSkeleton } from "../components/ui";
import { useLanguage } from "../i18n";
import { useAppState } from "../state";
import type { IntegrationName, IntegrationTest } from "../types";

interface HealthCheck {
  id?: IntegrationName;
  label: string;
  value: string;
  ok: boolean;
  testable?: boolean;
}

export function SystemPage() {
  const { health, email, loading } = useAppState();
  const { pick } = useLanguage();
  const [testing, setTesting] = useState<IntegrationName>();
  const [results, setResults] = useState<Partial<Record<IntegrationName, IntegrationTest>>>({});
  const [errors, setErrors] = useState<Partial<Record<IntegrationName, string>>>({});
  if (loading) return <PageSkeleton />;

  const checks: HealthCheck[] = [
    { label: pick("网站服务", "Web service"), value: health?.status === "ok" ? pick("正常", "Healthy") : pick("不可用", "Unavailable"), ok: health?.status === "ok" },
    { label: pick("任务调度", "Scheduler"), value: health?.status === "ok" ? pick("运行中", "Running") : pick("检查中", "Checking"), ok: health?.status === "ok" },
    { id: "database" as const, label: pick("数据库", "Database"), value: health?.storage || pick("未知", "Unknown"), ok: Boolean(health?.storage), testable: true },
    { id: "deepseek" as const, label: pick("AI 模型", "AI model"), value: health?.provider_configured ? health.model : pick("未配置", "Not configured"), ok: Boolean(health?.provider_configured), testable: Boolean(health?.provider_configured) },
    { id: "gemini" as const, label: pick("备用模型", "Fallback model"), value: health?.fallback_configured ? health.fallback_model : pick("未配置", "Not configured"), ok: Boolean(health?.fallback_configured), testable: Boolean(health?.fallback_configured) },
    { id: "tavily" as const, label: pick("联网检索", "Web research"), value: health?.web_search_status === "healthy" ? health.web_search_provider : health?.web_search_status === "unavailable" ? pick("暂时不可用", "Temporarily unavailable") : pick("未启用", "Disabled"), ok: health?.web_search_status === "healthy", testable: Boolean(health?.web_search_configured) },
    { id: "email" as const, label: pick("邮件服务", "Email"), value: email?.configured ? pick("已连接", "Connected") : pick("未配置", "Not configured"), ok: Boolean(email?.configured), testable: Boolean(email?.configured) },
  ];

  async function runTest(name: IntegrationName) {
    setTesting(name);
    setErrors((current) => ({ ...current, [name]: undefined }));
    try {
      const result = await api.testIntegration(name);
      setResults((current) => ({ ...current, [name]: result }));
    } catch (error) {
      setResults((current) => ({ ...current, [name]: undefined }));
      const message = error instanceof Error && error.message ? error.message : pick("连接测试失败", "Connection test failed");
      setErrors((current) => ({ ...current, [name]: message }));
    } finally {
      setTesting(undefined);
    }
  }

  return (
    <div className="system-page">
      <section className="system-health panel">
        <header className="panel-header"><div><h2>{pick("运行状态与真实连接测试", "Runtime status and live checks")}</h2><p>{health?.relay_configured ? pick("Tavily 与 Gemini 正通过 Cloudflare Relay 连接", "Tavily and Gemini are routed through Cloudflare Relay") : pick("当前使用服务直连", "Providers are connected directly")}</p></div><span className={`overall-health ${health?.status === "ok" ? "healthy" : ""}`}><i />{health?.status === "ok" ? pick("系统运行正常", "All systems operational") : pick("系统需要检查", "System needs attention")}</span></header>
        <div className="health-list">{checks.map((check) => <div className="health-item" key={check.label}><span className={`check-icon ${check.ok ? "ok" : "waiting"}`}>{check.ok ? <Check size={14} /> : <CircleDashed size={14} />}</span><strong>{check.label}</strong><code>{check.value}</code>{check.id && check.testable && <button className="integration-test" type="button" disabled={Boolean(testing)} onClick={() => void runTest(check.id!)}>{testing === check.id ? <LoaderCircle className="spin" size={12} /> : <Play size={11} />}{testing === check.id ? pick("测试中", "Testing") : pick("真实测试", "Live test")}</button>}{check.id && results[check.id] && <small className="test-success">{pick("成功", "Passed")} · {results[check.id]?.duration_ms} ms</small>}{check.id && errors[check.id] && <small className="test-failure" title={errors[check.id]}>{errors[check.id]}</small>}</div>)}</div>
        <p className="integration-note">{pick("提示：Tavily 测试会消耗一次搜索额度；邮件测试会向当前账号邮箱发送一封真实邮件。", "Note: the Tavily check uses one search credit; the email check sends a real message to your account address.")}</p>
      </section>
      <section className="build-info"><div><small>{pick("部署方式", "Deployment")}</small><strong>Docker</strong></div><div><small>{pick("数据库", "Database")}</small><strong>{health?.storage || "—"}</strong></div><div><small>{pick("默认时区", "Default time zone")}</small><strong>{health?.timezone || "Asia/Shanghai"}</strong></div><a href="https://github.com/chuanye-gao/CronPilot" target="_blank" rel="noreferrer">GitHub <ExternalLink size={14} /></a></section>
    </div>
  );
}
