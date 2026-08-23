import { ArrowLeft, ArrowRight, LockKeyhole, Mail } from "lucide-react";
import { useEffect, useState, type FormEvent } from "react";
import { Link, Navigate, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { Brand, LanguageToggle } from "../components/ui";
import { localizeError, useLanguage } from "../i18n";

export function AuthPage({ mode }: { mode: "login" | "register" }) {
  const navigate = useNavigate();
  const location = useLocation();
  const { user, loading: authLoading, login, register } = useAuth();
  const { language, pick } = useLanguage();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string>();
  const [registered, setRegistered] = useState(false);
  const registering = mode === "register";

  useEffect(() => {
    setError(undefined);
    setRegistered(false);
    setSubmitting(false);
    setPassword("");
    if (!registering) setName("");
  }, [mode, registering]);

  if (!authLoading && user) return <Navigate to="/app/overview" replace />;

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);
    setError(undefined);
    try {
      if (registering) {
        await register(name, email, password);
        setRegistered(true);
      } else {
        await login(email, password);
        const requested = (location.state as { from?: string } | null)?.from;
        navigate(requested?.startsWith("/app") ? requested : "/app/overview", { replace: true });
      }
    } catch (caught) {
      setError(localizeError(caught, language, pick("请求失败，请稍后重试。", "Request failed. Please try again.")));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="auth-page">
      <aside className="auth-story">
        <Brand />
        <div><h1>{registering ? pick("创建你的自动化工作区。", "Create your automation workspace.") : pick("欢迎回到 CronPilot。", "Welcome back to CronPilot.")}</h1><p>{pick("创建任务、按时运行，并在一个地方查看结果。", "Create tasks, run them on schedule, and review results in one place.")}</p></div>
      </aside>
      <main className="auth-panel">
        <Link className="auth-back" to="/"><ArrowLeft size={15} />{pick("返回首页", "Back to home")}</Link>
        <div className="auth-language"><LanguageToggle /></div>
        <div className="auth-card">
          <span className="auth-icon">{registering ? <Mail /> : <LockKeyhole />}</span>
          <h2>{registering ? pick("创建账户", "Create account") : pick("登录", "Sign in")}</h2>
          <p>{registering ? pick("验证邮箱后即可开始创建任务。", "Verify your email, then start creating tasks.") : pick("输入账户信息继续。", "Enter your account details to continue.")}</p>
          {registered ? (
            <div className="verification-sent"><Mail size={20} /><div><strong>{pick("验证邮件已经发送", "Verification email sent")}</strong><p>{pick(`请打开 ${email} 收到的邮件并完成验证。`, `Open the email sent to ${email} and verify your address.`)}</p><Link className="button button-dark" to="/login">{pick("前往登录", "Go to sign in")} <ArrowRight size={16} /></Link></div></div>
          ) : (
            <form onSubmit={submit}>
              {registering && <label className="field"><span>{pick("名称", "Name")}</span><input value={name} onChange={(event) => setName(event.target.value)} required maxLength={80} placeholder={pick("你的名字", "Your name")} autoComplete="name" /></label>}
              <label className="field"><span>{pick("邮箱", "Email")}</span><input value={email} onChange={(event) => setEmail(event.target.value)} type="email" required placeholder="you@example.com" autoComplete="email" /></label>
              <label className="field"><span>{pick("密码", "Password")}</span><input value={password} onChange={(event) => setPassword(event.target.value)} type="password" required minLength={8} maxLength={256} placeholder={pick("至少 8 位字符", "At least 8 characters")} autoComplete={registering ? "new-password" : "current-password"} /></label>
              {error && <div className="auth-message error">{error}</div>}
              <button className="button button-dark auth-submit" disabled={submitting}>{submitting ? pick("请稍候……", "Please wait…") : registering ? pick("创建账户", "Create account") : pick("登录", "Sign in")}<ArrowRight size={16} /></button>
            </form>
          )}
          <div className="auth-switch">{registering ? pick("已经有账户？", "Already have an account?") : pick("还没有账户？", "New to CronPilot?")}<Link to={registering ? "/login" : "/register"}>{registering ? pick("去登录", "Sign in") : pick("创建账户", "Create account")}</Link></div>
        </div>
      </main>
    </div>
  );
}
