import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

export type Language = "zh" | "en";

const chineseErrors: Array<[RegExp, string]> = [
  [/^invalid email address or password$/i, "邮箱或密码不正确。"],
  [/^email address has not been verified$/i, "邮箱尚未验证，请先完成邮箱验证。"],
  [/^email address is already registered$/i, "该邮箱已经注册，请直接登录。"],
  [/^verification link is invalid or expired$/i, "验证链接无效或已过期，请重新注册获取新链接。"],
  [/^authentication required$/i, "登录状态已失效，请重新登录。"],
  [/^request origin is not allowed$/i, "请求来源不受信任，请刷新页面后重试。"],
  [/^method not allowed$/i, "当前操作方式不受支持。"],
  [/^task not found$/i, "任务不存在或已经被删除。"],
  [/^execution not found$/i, "运行记录不存在。"],
  [/invalid schedule/i, "Cron 表达式无效，请使用五段格式，例如 0 8 * * *。"],
  [/invalid timezone/i, "时区无效，请填写 IANA 时区，例如 Asia\/Shanghai。"],
  [/timeout must be positive/i, "最长运行时间必须大于 0。"],
  [/retry\.max_attempts/i, "最多尝试次数必须在 1 到 10 之间。"],
  [/invalid delivery email address/i, "邮件通知的收件地址无效。"],
  [/email delivery is not configured/i, "邮件服务尚未配置，请配置 SMTP 后重启 CronPilot。"],
  [/^invalid request:/i, "请求内容格式不正确，请检查后重试。"],
  [/^internal server error/i, "服务内部发生错误，请稍后重试。"],
];

export function localizeError(error: unknown, language: Language, fallback: string) {
  if (!(error instanceof Error)) return fallback;
  if (language === "en") return error.message || fallback;
  const errorID = error.message.match(/\s+\((err_[^)]+)\)$/)?.[1];
  const message = error.message.replace(/\s+\(err_[^)]+\)$/, "");
  const translated = chineseErrors.find(([pattern]) => pattern.test(message))?.[1];
  const localized = translated || (/^[\x00-\x7F]*$/.test(message) ? fallback : message);
  return errorID ? `${localized}（错误编号：${errorID}）` : localized;
}

interface LanguageContextValue {
  language: Language;
  setLanguage: (language: Language) => void;
  pick: (chinese: string, english: string) => string;
}

const LanguageContext = createContext<LanguageContextValue | undefined>(undefined);
const storageKey = "cronpilot-language";

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [language, setLanguage] = useState<Language>(() => {
    const saved = window.localStorage.getItem(storageKey);
    if (saved === "zh" || saved === "en") return saved;
    return "zh";
  });

  useEffect(() => {
    window.localStorage.setItem(storageKey, language);
    document.documentElement.lang = language === "zh" ? "zh-CN" : "en";
  }, [language]);

  const value = useMemo<LanguageContextValue>(() => ({
    language,
    setLanguage,
    pick: (chinese, english) => language === "zh" ? chinese : english,
  }), [language]);

  return <LanguageContext.Provider value={value}>{children}</LanguageContext.Provider>;
}

export function useLanguage() {
  const context = useContext(LanguageContext);
  if (!context) throw new Error("useLanguage must be used inside LanguageProvider");
  return context;
}
