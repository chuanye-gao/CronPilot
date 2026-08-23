import { ArrowRight, Check, CircleAlert, LoaderCircle, MailCheck } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useAuth } from "../auth";
import { Brand, LanguageToggle } from "../components/ui";
import { localizeError, useLanguage } from "../i18n";

export function VerifyEmailPage() {
  const [search] = useSearchParams();
  const { verifyEmail } = useAuth();
  const { language, pick } = useLanguage();
  const started = useRef(false);
  const [state, setState] = useState<"working" | "success" | "error">("working");
  const [messageKey, setMessageKey] = useState<"working" | "missing" | "success" | "api-error">("working");
  const [apiError, setAPIError] = useState<unknown>();

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    const token = search.get("token") || "";
    if (!token) {
      setState("error");
      setMessageKey("missing");
      return;
    }
    void verifyEmail(token).then(() => {
      setState("success");
      setMessageKey("success");
    }).catch((error) => {
      setState("error");
      setAPIError(error);
      setMessageKey("api-error");
    });
  }, [search, verifyEmail]);

  const message = messageKey === "working" ? pick("正在验证你的邮箱地址……", "Verifying your email address…")
    : messageKey === "missing" ? pick("验证链接缺少必要信息。", "The verification link is incomplete.")
      : messageKey === "success" ? pick("邮箱验证成功，现在可以登录 CronPilot。", "Email verified. You can now sign in.")
        : localizeError(apiError, language, pick("验证链接无效或已过期。", "The verification link is invalid or expired."));

  return <div className="verify-page"><div className="verify-language"><LanguageToggle /></div><div className="verify-card"><Brand /><span className={`verify-icon ${state}`}>{state === "working" ? <LoaderCircle /> : state === "success" ? <MailCheck /> : <CircleAlert />}</span><h1>{state === "working" ? pick("验证邮箱", "Verify email") : state === "success" ? pick("验证完成", "Email verified") : pick("无法完成验证", "Verification failed")}</h1><p>{message}</p>{state === "success" && <Link className="button button-dark" to="/login"><Check size={16} />{pick("登录", "Sign in")} <ArrowRight size={16} /></Link>}{state === "error" && <Link className="button button-ghost" to="/register">{pick("返回注册", "Back to registration")}</Link>}</div></div>;
}
