import { CheckCircle2, KeyRound, MailCheck, Send, ShieldCheck } from "lucide-react";
import { useState, type FormEvent } from "react";
import { api } from "../api";
import { InlineError, PageSkeleton } from "../components/ui";
import { localizeError, useLanguage } from "../i18n";
import { useAppState } from "../state";

export function DeliveryPage() {
  const { email, loading } = useAppState();
  const { language, pick } = useLanguage();
  const [address, setAddress] = useState("");
  const [sending, setSending] = useState(false);
  const [result, setResult] = useState<string>();
  const [error, setError] = useState<string>();
  if (loading) return <PageSkeleton />;

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSending(true); setError(undefined); setResult(undefined);
    try {
      await api.testEmail(address);
      setResult(pick(`测试邮件已经发送到 ${address}`, `Test email sent to ${address}`));
    } catch (caught) {
      setError(localizeError(caught, language, pick("邮件发送失败", "Could not send the email")));
    } finally { setSending(false); }
  }

  return (
    <div className="delivery-layout">
      <section className="channel-card primary-channel">
        <header><span className="channel-logo"><MailCheck /></span><span className={`channel-status ${email?.configured ? "connected" : "pending"}`}><i />{email?.configured ? pick("已连接", "Connected") : pick("未配置", "Not configured")}</span></header>
        <h2>{pick("邮件通知", "Email delivery")}</h2><p>{pick("任务结束后，把结果发送到你指定的邮箱。", "Send task results to the email addresses you choose.")}</p>
        <dl><div><dt>{pick("发送方式", "Provider")}</dt><dd>{email?.provider || "SMTP"}</dd></div><div><dt>{pick("凭证位置", "Credentials")}</dt><dd>{pick("仅保存在服务端", "Server only")}</dd></div></dl>
        <div className="security-note"><ShieldCheck size={18} /><span><strong>{pick("凭证不会发送到浏览器", "Credentials stay on the server")}</strong>{pick("发件密码仅由 Go 后端读取。", "Only the Go backend can read the sending password.")}</span></div>
      </section>
      <section className="channel-card test-channel">
        <h2>{pick("发送测试邮件", "Send a test email")}</h2><p>{pick("输入一个收件地址，确认邮件服务可以正常送达。", "Enter a recipient to confirm that email delivery works.")}</p>
        <form onSubmit={submit}><label className="field"><span>{pick("收件邮箱", "Recipient")}</span><input type="email" required value={address} onChange={(event) => setAddress(event.target.value)} placeholder="you@example.com" /></label><button className="button button-primary" disabled={sending || !email?.configured}><Send size={15} />{sending ? pick("发送中…", "Sending…") : pick("发送测试", "Send test")}</button></form>
        {!email?.configured && <div className="setup-guide"><KeyRound size={17} /><span><strong>{pick("邮件服务尚未配置", "Email is not configured")}</strong>{pick("配置容器环境变量并重启 CronPilot 后即可使用。", "Configure the container environment and restart CronPilot.")}</span></div>}
        {result && <div className="success-message"><CheckCircle2 size={16} />{result}</div>}
        <InlineError message={error} />
      </section>
    </div>
  );
}
