import { Bot, Check, CircleAlert, Clock3, Languages, LoaderCircle, OctagonX, Radio, Square } from "lucide-react";
import { Link } from "react-router-dom";
import { useLanguage } from "../i18n";
import type { ExecutionStatus } from "../types";
import { statusText } from "../utils";

export function Brand({ link = "/", onClick }: { link?: string; onClick?: () => void }) {
  const { pick } = useLanguage();
  return (
    <Link className="brand" to={link} onClick={onClick} aria-label={pick("CronPilot 首页", "CronPilot home")}>
      <span className="brand-symbol"><i /><i /><i /></span>
      <span>CronPilot</span>
    </Link>
  );
}

export function LanguageToggle() {
  const { language, setLanguage, pick } = useLanguage();
  return (
    <div className="language-toggle" role="group" aria-label={pick("切换语言", "Change language")}>
      <Languages size={16} />
      <button className={language === "zh" ? "active" : ""} onClick={() => setLanguage("zh")} type="button">中</button>
      <span>/</span>
      <button className={language === "en" ? "active" : ""} onClick={() => setLanguage("en")} type="button">EN</button>
    </div>
  );
}

export function StatusBadge({ status }: { status: ExecutionStatus }) {
  const { language } = useLanguage();
  const Icon = status === "success" ? Check
    : status === "failed" ? OctagonX
      : status === "timeout" ? Clock3
        : status === "interrupted" ? Square
          : status === "running" ? LoaderCircle
            : Radio;
  return <span className={`status-badge status-${status}`}><Icon size={12} />{statusText(status, language)}</span>;
}

export function EmptyState({ title, body, action }: { title: string; body: string; action?: React.ReactNode }) {
  return (
    <div className="empty-state">
      <span className="empty-icon"><Bot size={24} /></span>
      <h3>{title}</h3>
      <p>{body}</p>
      {action}
    </div>
  );
}

export function InlineError({ message }: { message?: string }) {
  if (!message) return null;
  return <div className="inline-error" role="alert"><CircleAlert size={15} />{message}</div>;
}

export function InlineSuccess({ message }: { message?: string }) {
  if (!message) return null;
  return <div className="inline-success" role="status"><Check size={15} />{message}</div>;
}

export function LinkifiedOutput({ value, className }: { value: string; className?: string }) {
  const parts: React.ReactNode[] = [];
  const links = /\[([^\]\n]+)\]\((https?:\/\/[^\s)]+)\)|(https?:\/\/[^\s<]+)/g;
  let cursor = 0;
  let match: RegExpExecArray | null;
  while ((match = links.exec(value)) !== null) {
    if (match.index > cursor) parts.push(value.slice(cursor, match.index));
    let href = match[2] || match[3];
    let label = match[1] || href;
    let trailing = "";
    if (!match[2]) {
      while (/[),.;!?，。；：！？]$/.test(href)) {
        trailing = href.slice(-1) + trailing;
        href = href.slice(0, -1);
      }
      label = href;
    }
    parts.push(<a href={href} key={`${match.index}-${href}`} target="_blank" rel="noreferrer">{label}</a>);
    if (trailing) parts.push(trailing);
    cursor = match.index + match[0].length;
  }
  if (cursor < value.length) parts.push(value.slice(cursor));
  return <pre className={className}>{parts}</pre>;
}

export function PageSkeleton() {
  return <div className="page-skeleton"><span /><span /><span /></div>;
}
