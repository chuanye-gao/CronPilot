import { useEffect, useState, type FormEvent } from "react";
import { BellRing, Bot, CalendarClock, Settings2, X } from "lucide-react";
import { localizeError, useLanguage } from "../i18n";
import type { Task, TaskInput } from "../types";
import { InlineError } from "./ui";
import { TaskAssistantDialog } from "./TaskAssistantDialog";

interface Props {
  open: boolean;
  task?: Task;
  onClose: () => void;
  onSave: (value: TaskInput) => Promise<void>;
}

const emptyTask: TaskInput = {
  name: "",
  description: "",
  schedule: "0 8 * * *",
  timezone: "Asia/Shanghai",
  prompt: "",
  enabled: true,
  timeout: "5m",
  retry: { max_attempts: 1, delay: "10s" },
  delivery: {},
};

export function TaskDialog({ open, task, onClose, onSave }: Props) {
  const { language, pick } = useLanguage();
  const [value, setValue] = useState<TaskInput>(emptyTask);
  const [emailMode, setEmailMode] = useState("off");
  const [recipients, setRecipients] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();

  useEffect(() => {
    if (!open) return;
    if (!task) {
      setValue(emptyTask);
      setEmailMode("off");
      setRecipients("");
    } else {
      const { id: _id, created_at: _created, updated_at: _updated, next_run: _next, ...input } = task;
      setValue(input);
      const events = task.delivery?.on || [];
      setEmailMode(!task.delivery?.type ? "off" : events.length === 1 && events[0] === "success" ? "success" : events.includes("success") ? "all" : "failures");
      setRecipients((task.delivery?.to || []).join(", "));
    }
    setError(undefined);
  }, [open, task]);

  if (!open) return null;

  if (!task) return <TaskAssistantDialog onClose={onClose} onSave={onSave} />;

  async function submit(event: FormEvent) {
    event.preventDefault();
    const emails = recipients.split(",").map((item) => item.trim()).filter(Boolean);
    if (emailMode !== "off" && emails.length === 0) {
      setError(pick("启用邮件通知时，请至少填写一个收件地址。", "Add at least one recipient when email delivery is enabled."));
      return;
    }
    const events = emailMode === "all" ? ["success", "failed", "timeout"]
      : emailMode === "failures" ? ["failed", "timeout"]
        : emailMode === "success" ? ["success"] : [];
    const input: TaskInput = {
      ...value,
      delivery: emailMode === "off" ? {} : { type: "email", to: emails, on: events, include_output: true },
    };
    setSaving(true);
    setError(undefined);
    try {
      await onSave(input);
      onClose();
    } catch (caught) {
      setError(localizeError(caught, language, pick("保存失败", "Could not save the task")));
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section className="task-modal" role="dialog" aria-modal="true" aria-labelledby="task-modal-title">
        <header className="modal-header">
          <div><h2 id="task-modal-title">{task ? pick("编辑任务", "Edit task") : pick("创建任务", "Create task")}</h2></div>
          <button className="icon-button" type="button" onClick={onClose} aria-label={pick("关闭", "Close")}><X size={18} /></button>
        </header>
        <form onSubmit={submit}>
          <div className="task-form-sections">
            <fieldset className="task-form-section">
              <legend><Bot size={16} /><span>{pick("任务内容", "Task content")}<small>{pick("说明任务目标和 AI 要完成的工作", "Define the goal and the work for AI")}</small></span></legend>
              <div className="form-grid">
                <label className="field field-wide"><span>{pick("任务名称", "Task name")}</span><input required maxLength={100} value={value.name} onChange={(event) => setValue({ ...value, name: event.target.value })} placeholder={pick("每日 AI 新闻简报", "Daily AI news brief")} /></label>
                <label className="field field-wide"><span>{pick("说明", "Description")}</span><input maxLength={240} value={value.description} onChange={(event) => setValue({ ...value, description: event.target.value })} placeholder={pick("这项任务会完成什么", "What should this task deliver?")} /></label>
                <label className="field field-wide"><span>{pick("给 AI 的指令", "Instructions for AI")}</span><textarea required rows={6} value={value.prompt} onChange={(event) => setValue({ ...value, prompt: event.target.value })} placeholder={pick("清楚描述 AI 需要完成的工作……", "Describe the work clearly…")} /></label>
              </div>
            </fieldset>

            <fieldset className="task-form-section">
              <legend><CalendarClock size={16} /><span>{pick("计划与状态", "Schedule and status")}<small>{pick("设置运行时间、时区和自动执行状态", "Choose when and where the task runs")}</small></span></legend>
              <div className="form-grid">
                <label className="field"><span>{pick("执行计划", "Schedule")}</span><input required value={value.schedule} onChange={(event) => setValue({ ...value, schedule: event.target.value })} /><small>{pick("Cron 五段表达式，例如 0 8 * * *", "Five-field cron, e.g. 0 8 * * *")}</small></label>
                <label className="field"><span>{pick("时区", "Time zone")}</span><input required value={value.timezone} onChange={(event) => setValue({ ...value, timezone: event.target.value })} /></label>
                <label className="switch-field field-wide"><input type="checkbox" checked={value.enabled ?? true} onChange={(event) => setValue({ ...value, enabled: event.target.checked })} /><span className="switch-track" /><span><strong>{pick("启用任务", "Enable task")}</strong><small>{pick("按计划自动运行", "Run automatically")}</small></span></label>
              </div>
            </fieldset>

            <fieldset className="task-form-section">
              <legend><Settings2 size={16} /><span>{pick("可靠性", "Reliability")}<small>{pick("限制运行时间并控制失败重试", "Control timeout and retry behavior")}</small></span></legend>
              <div className="form-grid reliability-grid">
                <label className="field"><span>{pick("最长运行时间", "Timeout")}</span><input required value={value.timeout} onChange={(event) => setValue({ ...value, timeout: event.target.value })} /></label>
                <label className="field"><span>{pick("最多尝试次数", "Max attempts")}</span><input required type="number" min={1} max={10} value={value.retry.max_attempts} onChange={(event) => setValue({ ...value, retry: { ...value.retry, max_attempts: Number(event.target.value) } })} /></label>
                <label className="field"><span>{pick("重试间隔", "Retry delay")}</span><input required value={value.retry.delay} onChange={(event) => setValue({ ...value, retry: { ...value.retry, delay: event.target.value } })} /></label>
              </div>
            </fieldset>

            <fieldset className="task-form-section">
              <legend><BellRing size={16} /><span>{pick("邮件通知", "Email delivery")}<small>{pick("选择发送条件和收件地址", "Choose delivery events and recipients")}</small></span></legend>
              <div className="form-grid compact-grid">
                <label className="field"><span>{pick("什么时候发送", "Send when")}</span><select value={emailMode} onChange={(event) => setEmailMode(event.target.value)}><option value="off">{pick("不发送", "Off")}</option><option value="all">{pick("任何结果", "Any result")}</option><option value="failures">{pick("失败或超时", "Failed or timed out")}</option><option value="success">{pick("成功", "Succeeded")}</option></select></label>
                <label className="field"><span>{pick("收件邮箱", "Recipients")}</span><input type="email" multiple disabled={emailMode === "off"} value={recipients} onChange={(event) => setRecipients(event.target.value)} placeholder="you@example.com" /></label>
              </div>
            </fieldset>
          </div>
          <InlineError message={error} />
          <footer className="modal-actions"><button className="button button-ghost" type="button" onClick={onClose}>{pick("取消", "Cancel")}</button><button className="button button-primary" disabled={saving}>{saving ? pick("保存中…", "Saving…") : task ? pick("保存修改", "Save changes") : pick("创建任务", "Create task")}</button></footer>
        </form>
      </section>
    </div>
  );
}
