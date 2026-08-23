import { useEffect, useState } from "react";
import { Activity, BellRing, Blocks, CircleGauge, LogOut, Menu, Plus, ServerCog, X } from "lucide-react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { TaskDialog } from "../components/TaskDialog";
import { Brand, InlineError, InlineSuccess, LanguageToggle } from "../components/ui";
import { useLanguage } from "../i18n";
import { useAppState } from "../state";
import type { Task, TaskInput } from "../types";

export interface ConsoleOutletContext {
  openTaskDialog: (task?: Task) => void;
}

export function ConsoleLayout() {
  const location = useLocation();
  const navigate = useNavigate();
  const { user, logout } = useAuth();
  const { pick } = useLanguage();
  const { createTask, updateTask, error, health, tasks } = useAppState();
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<Task>();
  const [notice, setNotice] = useState<string>();
  const navigation = [
    { to: "/app/overview", label: pick("总览", "Overview"), icon: CircleGauge },
    { to: "/app/tasks", label: pick("任务", "Tasks"), icon: Blocks },
    { to: "/app/runs", label: pick("运行记录", "Runs"), icon: Activity },
    { to: "/app/delivery", label: pick("邮件通知", "Email"), icon: BellRing },
    { to: "/app/system", label: pick("系统状态", "System"), icon: ServerCog },
  ];
  const pageTitles: Record<string, string> = {
    "/app/overview": pick("总览", "Overview"), "/app/tasks": pick("任务管理", "Tasks"),
    "/app/runs": pick("运行记录", "Runs"), "/app/delivery": pick("邮件通知", "Email delivery"),
    "/app/system": pick("系统状态", "System status"),
  };
  const pageTitle = pageTitles[location.pathname] || pageTitles["/app/overview"];
  const showCreate = location.pathname === "/app/overview" || location.pathname === "/app/tasks";

  useEffect(() => { setSidebarOpen(false); }, [location.pathname]);
  useEffect(() => {
    if (!notice) return;
    const timer = window.setTimeout(() => setNotice(undefined), 4_000);
    return () => window.clearTimeout(timer);
  }, [notice]);

  function openTaskDialog(task?: Task) {
    setEditingTask(task);
    setDialogOpen(true);
  }

  async function saveTask(value: TaskInput) {
    if (editingTask) {
      await updateTask(editingTask.id, value);
      setNotice(pick("任务修改已保存。", "Task changes saved."));
    } else {
      await createTask(value);
      setNotice(pick("任务创建成功。", "Task created."));
    }
  }

  return (
    <div className="console-shell">
      <aside className={`console-sidebar ${sidebarOpen ? "open" : ""}`}>
        <div className="sidebar-brand"><Brand link="/app/overview" onClick={() => setSidebarOpen(false)} /><button className="icon-button sidebar-close" onClick={() => setSidebarOpen(false)} aria-label={pick("关闭菜单", "Close menu")} title={pick("关闭菜单", "Close menu")}><X size={18} /></button></div>
        <nav className="console-nav">
          {navigation.map(({ to, label, icon: Icon }) => <NavLink key={to} to={to} onClick={() => setSidebarOpen(false)}><Icon size={17} /><span>{label}</span>{to === "/app/tasks" && <em>{tasks.length}</em>}</NavLink>)}
        </nav>
        <div className="sidebar-runtime">
          <div className="runtime-head"><span className={`online-dot ${health?.status === "ok" ? "online" : ""}`} /><strong>{health?.status === "ok" ? pick("系统运行正常", "System operational") : pick("正在检查系统", "Checking system")}</strong></div>
          <p>{health?.storage === "sqlite" ? pick("数据已安全保存", "Data is persisted") : pick("正在检查存储", "Checking storage")}</p>
        </div>
        <div className="sidebar-account"><span className="account-avatar">{user?.name?.slice(0, 1).toUpperCase() || "U"}</span><span><strong>{user?.name}</strong><small>{user?.email}</small></span><button className="icon-button" aria-label={pick("退出登录", "Log out")} title={pick("退出登录", "Log out")} onClick={() => void logout().then(() => navigate("/login", { replace: true }))}><LogOut size={14} /></button></div>
      </aside>
      {sidebarOpen && <button className="sidebar-scrim" onClick={() => setSidebarOpen(false)} aria-label={pick("关闭菜单", "Close menu")} />}
      <main className="console-main">
        <header className="console-topbar">
          <button className="icon-button mobile-menu" onClick={() => setSidebarOpen(true)} aria-label={pick("打开菜单", "Open menu")} title={pick("打开菜单", "Open menu")} aria-expanded={sidebarOpen}><Menu size={19} /></button>
          <div><h1>{pageTitle}</h1></div>
          <div className="topbar-actions">
            <LanguageToggle />
            {showCreate && <button className="button button-primary" onClick={() => openTaskDialog()}><Plus size={16} />{pick("新建任务", "New task")}</button>}
          </div>
        </header>
        <div className="console-content">
          <InlineError message={error} />
          <InlineSuccess message={notice} />
          <Outlet context={{ openTaskDialog } satisfies ConsoleOutletContext} />
        </div>
      </main>
      <TaskDialog open={dialogOpen} task={editingTask} onClose={() => setDialogOpen(false)} onSave={saveTask} />
    </div>
  );
}
