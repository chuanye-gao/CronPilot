import { Bot, Check, ChevronDown, Clock3, Mail, Play, Send, Sparkles, UserRound, WandSparkles, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import { api } from "../api";
import { useAuth } from "../auth";
import { localizeError, useLanguage } from "../i18n";
import { useAppState } from "../state";
import type { AssistantMessage, TaskAssistantDraft, TaskInput } from "../types";
import { InlineError, LinkifiedOutput } from "./ui";

interface Props {
  onClose: () => void;
  onSave: (value: TaskInput) => Promise<void>;
}

function emailEvents(value: TaskAssistantDraft["email_events"]) {
  if (value === "failures") return ["failed", "timeout"];
  if (value === "success") return ["success"];
  return ["success", "failed", "timeout"];
}

export function TaskAssistantDialog({ onClose, onSave }: Props) {
  const { user } = useAuth();
  const { health } = useAppState();
  const { language, pick } = useLanguage();
  const [messages, setMessages] = useState<AssistantMessage[]>([{
    role: "assistant",
    content: pick("告诉我你希望定期得到什么结果。我会帮你理清需求、设置时间并写好给 AI 的指令；需要实时信息时，任务会自动联网检索并附上来源。", "Tell me what result you want on a recurring basis. I’ll clarify the goal, set the schedule, and write the AI instructions; tasks automatically research the live web and cite sources when current information is needed."),
  }]);
  const [draft, setDraft] = useState<TaskAssistantDraft>({
    name: "", description: "", schedule: "", schedule_label: "", timezone: health?.timezone || "Asia/Shanghai",
    prompt: "", notify_email: false, email_events: "all", recipient: user?.email || "",
  });
  const [input, setInput] = useState("");
  const [quickReplies, setQuickReplies] = useState<string[]>([]);
  const [ready, setReady] = useState(false);
  const [sending, setSending] = useState(false);
  const [testing, setTesting] = useState(false);
  const [saving, setSaving] = useState(false);
  const [advanced, setAdvanced] = useState(false);
  const [testOutput, setTestOutput] = useState<string>();
  const [testedFingerprint, setTestedFingerprint] = useState("");
  const [error, setError] = useState<string>();
  const conversationEnd = useRef<HTMLDivElement>(null);
  const fingerprint = useMemo(() => JSON.stringify(draft), [draft]);
  const canTest = Boolean(draft.name.trim() && draft.schedule.trim() && draft.prompt.trim());
  const canCreate = Boolean(testOutput && testedFingerprint === fingerprint);
  const hasUserMessage = messages.some((message) => message.role === "user");
  const templates = language === "zh"
    ? ["每天整理 AI 行业的重要新闻", "每周总结我的项目进展", "定期跟踪一个 GitHub 项目"]
    : ["Summarize important AI news every day", "Create a weekly project review", "Track a GitHub project regularly"];

  useEffect(() => { conversationEnd.current?.scrollIntoView({ behavior: "smooth" }); }, [messages, sending]);

  function updateDraft(changes: Partial<TaskAssistantDraft>) {
    setDraft((current) => ({ ...current, ...changes }));
    setTestOutput(undefined);
    setTestedFingerprint("");
  }

  async function sendMessage(content: string) {
    content = content.trim();
    if (!content || sending) return;
    const nextMessages = [...messages, { role: "user", content } satisfies AssistantMessage].slice(-22);
    setMessages(nextMessages);
    setInput("");
    setQuickReplies([]);
    setSending(true);
    setError(undefined);
    try {
      const plan = await api.planTask(language, nextMessages, draft);
      setDraft(plan.draft);
      setReady(plan.ready);
      setQuickReplies(plan.quick_replies || []);
      setMessages([...nextMessages, { role: "assistant", content: plan.reply }]);
      setTestOutput(undefined);
      setTestedFingerprint("");
    } catch (caught) {
      setError(localizeError(caught, language, pick("AI 创建助手暂时无法响应", "The task assistant could not respond")));
    } finally {
      setSending(false);
    }
  }

  function submitMessage(event: FormEvent) {
    event.preventDefault();
    void sendMessage(input);
  }

  function handleComposerKey(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) {
      event.preventDefault();
      void sendMessage(input);
    }
  }

  async function testDraft() {
    setTesting(true);
    setError(undefined);
    setTestOutput(undefined);
    try {
      let result = await api.testTaskDraft(draft);
      const deadline = Date.now() + 5 * 60_000 + 10_000;
      while (result.status === "running" && Date.now() < deadline) {
        await new Promise((resolve) => window.setTimeout(resolve, 1_000));
        result = await api.taskDraftTest(result.id);
      }
      if (result.status !== "success" || !result.output) {
        throw new Error(result.error || pick("测试运行超时", "Test run timed out"));
      }
      setTestOutput(result.output);
      setTestedFingerprint(fingerprint);
    } catch (caught) {
      setError(localizeError(caught, language, pick("测试运行失败", "Test run failed")));
    } finally {
      setTesting(false);
    }
  }

  async function createTask() {
    if (!canCreate) return;
    setSaving(true);
    setError(undefined);
    try {
      const includeOutput = true;
      await onSave({
        name: draft.name.trim(), description: draft.description.trim(), schedule: draft.schedule.trim(),
        timezone: draft.timezone.trim(), prompt: draft.prompt.trim(), enabled: true, timeout: "5m",
        retry: { max_attempts: 1, delay: "10s" },
        delivery: draft.notify_email ? { type: "email", to: [draft.recipient.trim()], on: emailEvents(draft.email_events), include_output: includeOutput } : {},
      });
      onClose();
    } catch (caught) {
      setError(localizeError(caught, language, pick("创建任务失败", "Could not create the task")));
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section className="task-modal assistant-modal" role="dialog" aria-modal="true" aria-labelledby="assistant-title">
        <header className="modal-header assistant-header">
          <div><span className="assistant-title-icon"><WandSparkles size={18} /></span><span><h2 id="assistant-title">{pick("用 AI 创建任务", "Create with AI")}</h2><small>{pick("描述目标，CronPilot 帮你完成配置", "Describe the goal and CronPilot will configure it")}</small></span></div>
          <button className="icon-button" type="button" onClick={onClose} aria-label={pick("关闭", "Close")}><X size={19} /></button>
        </header>

        <div className="assistant-builder">
          <section className="assistant-chat">
            <div className="assistant-messages">
              {messages.map((message, index) => <div className={`assistant-message ${message.role}`} key={`${message.role}-${index}`}><span>{message.role === "assistant" ? <Bot size={17} /> : <UserRound size={17} />}</span><p>{message.content}</p></div>)}
              {!hasUserMessage && <div className="assistant-starters"><small>{pick("可以这样开始", "Try one of these")}</small>{templates.map((template) => <button type="button" key={template} onClick={() => void sendMessage(template)}>{template}</button>)}</div>}
              {quickReplies.length > 0 && <div className="assistant-quick-replies">{quickReplies.map((reply) => <button type="button" key={reply} onClick={() => void sendMessage(reply)}>{reply}</button>)}</div>}
              {sending && <div className="assistant-message assistant thinking"><span><Bot size={17} /></span><p><i /><i /><i /></p></div>}
              <div ref={conversationEnd} />
            </div>
            <form className="assistant-composer" onSubmit={submitMessage}>
              <textarea rows={2} maxLength={4000} value={input} onChange={(event) => setInput(event.target.value)} onKeyDown={handleComposerKey} placeholder={pick("例如：每天早上给我一份 AI 新闻简报……", "For example: send me an AI news brief every morning…")} />
              <button className="composer-send" disabled={sending || !input.trim()} aria-label={pick("发送", "Send")}><Send size={18} /></button>
            </form>
          </section>

          <aside className="assistant-draft">
            <div className="draft-heading"><span><Sparkles size={16} /></span><div><h3>{pick("任务草稿", "Task draft")}</h3><p>{ready ? pick("信息已足够，可以测试", "Ready for a test run") : pick("会随着对话自动更新", "Updates as you chat")}</p></div><em className={ready ? "ready" : "building"}>{ready ? pick("可测试", "READY") : pick("整理中", "DRAFT")}</em></div>

            <div className="draft-fields">
              <label className="field"><span>{pick("任务名称", "Task name")}</span><input value={draft.name} onChange={(event) => updateDraft({ name: event.target.value })} placeholder={pick("由 AI 自动生成", "Generated by AI")} /></label>
              <div className="draft-summary-row"><span><Clock3 size={16} /></span><div><small>{pick("执行时间", "Schedule")}</small><strong>{draft.schedule_label || pick("等待确认", "Not set yet")}</strong></div></div>
              <label className="draft-email-switch"><input type="checkbox" checked={draft.notify_email} onChange={(event) => updateDraft({ notify_email: event.target.checked, recipient: draft.recipient || user?.email || "" })} /><span className="switch-track" /><span><Mail size={16} />{pick("运行结束后发邮件", "Email the result")}</span></label>
              {draft.notify_email && <label className="field"><span>{pick("收件邮箱", "Recipient")}</span><input type="email" value={draft.recipient} onChange={(event) => updateDraft({ recipient: event.target.value })} placeholder="you@example.com" /></label>}
            </div>

            <button className="advanced-toggle" type="button" onClick={() => setAdvanced((value) => !value)}>{pick("查看和调整高级设置", "Review advanced settings")}<ChevronDown className={advanced ? "open" : ""} size={16} /></button>
            {advanced && <div className="advanced-fields">
              <label className="field"><span>{pick("任务说明", "Description")}</span><input value={draft.description} onChange={(event) => updateDraft({ description: event.target.value })} /></label>
              <div className="advanced-grid"><label className="field"><span>Cron</span><input value={draft.schedule} onChange={(event) => updateDraft({ schedule: event.target.value })} /></label><label className="field"><span>{pick("时区", "Time zone")}</span><input value={draft.timezone} onChange={(event) => updateDraft({ timezone: event.target.value })} /></label></div>
              {draft.notify_email && <label className="field"><span>{pick("发送条件", "Send when")}</span><select value={draft.email_events} onChange={(event) => updateDraft({ email_events: event.target.value as TaskAssistantDraft["email_events"] })}><option value="all">{pick("任何结果", "Any result")}</option><option value="failures">{pick("失败或超时", "Failed or timed out")}</option><option value="success">{pick("成功", "Succeeded")}</option></select></label>}
              <label className="field"><span>{pick("给 AI 的完整指令", "Full AI instructions")}</span><textarea rows={8} value={draft.prompt} onChange={(event) => updateDraft({ prompt: event.target.value })} /></label>
            </div>}

            {testOutput && testedFingerprint === fingerprint && <div className="assistant-test-result"><header><Check size={15} />{pick("测试完成", "Test completed")}</header><LinkifiedOutput value={testOutput} /></div>}
            <InlineError message={error} />
            <div className="assistant-actions">
              <button className="button button-ghost" type="button" disabled={!canTest || testing || sending} onClick={() => void testDraft()}><Play size={16} />{testing ? pick("正在测试…", "Testing…") : pick("测试运行", "Test run")}</button>
              <button className="button button-primary" type="button" disabled={!canCreate || saving} onClick={() => void createTask()}>{saving ? pick("创建中…", "Creating…") : pick("创建任务", "Create task")}</button>
            </div>
            {!canCreate && <p className="assistant-action-hint">{canTest ? pick("先测试一次真实输出，确认后即可创建。", "Run one real test before creating the task.") : pick("继续沟通，任务信息完整后即可测试。", "Keep chatting until the draft is ready to test.")}</p>}
          </aside>
        </div>
      </section>
    </div>
  );
}
