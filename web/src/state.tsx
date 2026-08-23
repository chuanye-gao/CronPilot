import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { api } from "./api";
import { localizeError, useLanguage } from "./i18n";
import type { EmailStatus, Execution, Health, Task, TaskInput } from "./types";

interface AppState {
  health?: Health;
  email?: EmailStatus;
  tasks: Task[];
  runs: Execution[];
  loading: boolean;
  error?: string;
  refresh: () => Promise<void>;
  createTask: (value: TaskInput) => Promise<Task>;
  updateTask: (id: string, value: TaskInput) => Promise<Task>;
  deleteTask: (id: string) => Promise<void>;
  runTask: (id: string) => Promise<Execution>;
}

const StateContext = createContext<AppState | undefined>(undefined);

export function StateProvider({ children }: { children: ReactNode }) {
  const { language, pick } = useLanguage();
  const [health, setHealth] = useState<Health>();
  const [email, setEmail] = useState<EmailStatus>();
  const [tasks, setTasks] = useState<Task[]>([]);
  const [runs, setRuns] = useState<Execution[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();

  const refresh = useCallback(async () => {
    try {
      const [healthResult, taskResult, runResult, emailResult] = await Promise.all([
        api.health(),
        api.tasks(),
        api.runs(),
        api.emailStatus(),
      ]);
      setHealth(healthResult);
      setTasks(taskResult || []);
      setRuns(runResult || []);
      setEmail(emailResult);
      setError(undefined);
    } catch (caught) {
      setError(localizeError(caught, language, pick("无法连接 CronPilot", "Could not connect to CronPilot")));
    } finally {
      setLoading(false);
    }
  }, [language, pick]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (!runs.some((run) => run.status === "pending" || run.status === "running" || run.delivery_status === "pending")) return;
    const timer = window.setInterval(() => void refresh(), 3_000);
    return () => window.clearInterval(timer);
  }, [refresh, runs]);

  const value = useMemo<AppState>(() => ({
    health,
    email,
    tasks,
    runs,
    loading,
    error,
    refresh,
    createTask: async (input) => {
      const created = await api.createTask(input);
      await refresh();
      return created;
    },
    updateTask: async (id, input) => {
      const updated = await api.updateTask(id, input);
      await refresh();
      return updated;
    },
    deleteTask: async (id) => {
      await api.deleteTask(id);
      await refresh();
    },
    runTask: async (id) => {
      const run = await api.runTask(id);
      await refresh();
      return run;
    },
  }), [email, error, health, loading, refresh, runs, tasks]);

  return <StateContext.Provider value={value}>{children}</StateContext.Provider>;
}

export function useAppState() {
  const value = useContext(StateContext);
  if (!value) throw new Error("useAppState must be used inside StateProvider");
  return value;
}
