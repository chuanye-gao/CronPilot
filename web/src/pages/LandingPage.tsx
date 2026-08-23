import { ArrowRight, Blocks, Box, CheckCircle2, Github, Mail, Radar, ShieldCheck, TimerReset } from "lucide-react";
import type { MouseEvent } from "react";
import { Link } from "react-router-dom";
import { useAuth } from "../auth";
import { Brand, LanguageToggle } from "../components/ui";
import { useLanguage } from "../i18n";

export function LandingPage() {
  const { user, loading } = useAuth();
  const { pick } = useLanguage();
  function scrollToSection(event: MouseEvent<HTMLAnchorElement>, id: string) {
    event.preventDefault();
    document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" });
  }
  return (
    <div className="landing-page">
      <header className="public-header">
        <Brand />
        <nav><a href="#product" onClick={(event) => scrollToSection(event, "product")}>{pick("功能", "Features")}</a><a href="#architecture" onClick={(event) => scrollToSection(event, "architecture")}>{pick("部署", "Deployment")}</a><a href="https://github.com/chuanye-gao/CronPilot" target="_blank" rel="noreferrer">GitHub</a></nav>
        <div><LanguageToggle />{loading ? <span className="header-session-placeholder" aria-hidden="true" /> : user ? <Link className="button button-dark" to="/app/overview">{pick("进入控制台", "Open console")} <ArrowRight size={15} /></Link> : <><Link className="text-link" to="/login">{pick("登录", "Sign in")}</Link><Link className="button button-dark" to="/register">{pick("开始使用", "Get started")} <ArrowRight size={15} /></Link></>}</div>
      </header>

      <main>
        <section className="landing-hero">
          <div className="hero-copy">
            <h1>{pick("让 AI 任务，", "AI tasks,")}<br /><em>{pick("按时完成。", "right on time.")}</em></h1>
            <p>{pick("创建定时 AI 任务，查看每次运行，并在完成后收到邮件。", "Schedule AI tasks, review every run, and receive results by email.")}</p>
            <div className="hero-actions"><Link className="button button-acid" to="/app/overview">{pick("打开控制台", "Open console")} <ArrowRight size={16} /></Link><a className="button button-outline-light" href="https://github.com/chuanye-gao/CronPilot" target="_blank" rel="noreferrer"><Github size={16} />GitHub</a></div>
            <div className="hero-proof"><span><CheckCircle2 size={15} />{pick("数据保存在本机", "Your data stays local")}</span><span><CheckCircle2 size={15} />{pick("Docker 部署", "Docker deployment")}</span><span><CheckCircle2 size={15} />{pick("邮件通知", "Email delivery")}</span></div>
          </div>
          <div className="product-preview" aria-label={pick("CronPilot 产品预览", "CronPilot product preview")}>
            <div className="preview-window-bar"><span>cronpilot / workspace</span><i /><i /><i /></div>
            <div className="preview-shell">
              <aside><b>CP</b><i className="selected" /><i /><i /><i /></aside>
              <div className="preview-content">
                <h3>{pick("任务总览", "Task overview")}</h3>
                <div className="preview-metrics"><article><small>{pick("启用任务", "Active tasks")}</small><strong>08</strong><em>{pick("2 项已暂停", "2 paused")}</em></article><article><small>{pick("成功率", "Success rate")}</small><strong>98.4%</strong><em>{pick("最近 30 次", "Last 30 runs")}</em></article><article><small>{pick("下次运行", "Next run")}</small><strong>08:00</strong><em>{pick("每日简报", "Daily brief")}</em></article></div>
                <div className="preview-list"><header><strong>{pick("计划任务", "Scheduled tasks")}</strong><span>{pick("查看全部", "View all")}</span></header><div><i className="green" /><span><b>Daily AI Brief</b><small>{pick("每天 08:00", "Daily at 08:00")} · Asia/Shanghai</small></span><em>{pick("2 小时后", "in 2 hr")}</em></div><div><i className="purple" /><span><b>Repository Monitor</b><small>{pick("每 4 小时", "Every 4 hours")}</small></span><em>{pick("4 小时后", "in 4 hr")}</em></div></div>
              </div>
            </div>
          </div>
        </section>

        <section className="landing-section" id="product">
          <div className="section-intro"><h2>{pick("从创建到交付，", "From schedule to delivery,")}<br />{pick("每一步都看得见。", "every step is visible.")}</h2><p>{pick("CronPilot 专注于一件事：让 AI 任务长期、稳定、按计划运行。", "CronPilot focuses on one job: running AI tasks reliably on schedule.")}</p></div>
          <div className="feature-grid">
            <article><span><TimerReset /></span><h3>{pick("按时运行", "Run on schedule")}</h3><p>{pick("支持 Cron、时区、超时和失败重试。", "Cron schedules, time zones, timeouts, and retries.")}</p></article>
            <article><span><Radar /></span><h3>{pick("查看每次结果", "Review every run")}</h3><p>{pick("状态、耗时、输出和错误都保留下来。", "Keep status, duration, output, and errors.")}</p></article>
            <article><span><Mail /></span><h3>{pick("邮件通知", "Email results")}</h3><p>{pick("成功、失败或超时后自动发送结果。", "Deliver results after success, failure, or timeout.")}</p></article>
            <article><span><ShieldCheck /></span><h3>{pick("自己掌控数据", "Own your data")}</h3><p>{pick("密钥保留在服务端，数据存放在你的机器。", "Keys stay server-side and data stays on your machine.")}</p></article>
          </div>
        </section>

        <section className="architecture-section" id="architecture">
          <div><h2>{pick("一个容器，", "One container,")}<br />{pick("就是完整系统。", "the complete system.")}</h2><p>{pick("React 前端由 Go 直接提供，任务和运行记录保存在 SQLite 中。不需要 Redis。", "The Go service hosts the React app and stores tasks and runs in SQLite. Redis is not required.")}</p><Link className="text-arrow" to="/app/system">{pick("查看系统状态", "View system status")} <ArrowRight size={15} /></Link></div>
          <div className="architecture-flow"><span><Blocks />{pick("网页控制台", "Web console")}</span><i>→</i><span><Box />{pick("任务引擎", "Task engine")}</span><i>→</i><span><Radar />{pick("AI 模型", "AI model")}</span><i>→</i><span><Mail />{pick("邮件", "Email")}</span></div>
        </section>
      </main>
      <footer className="public-footer"><Brand /><p>{pick("让 AI 任务按时运行。", "AI work, right on time.")}</p><span>{pick("开源 · 自托管", "Open source · Self-hosted")}</span></footer>
    </div>
  );
}
