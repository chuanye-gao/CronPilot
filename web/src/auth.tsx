import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { api, APIError } from "./api";
import { useLanguage } from "./i18n";
import type { User } from "./types";

interface AuthState {
  user?: User;
  loading: boolean;
  login: (email: string, password: string) => Promise<User>;
  register: (name: string, email: string, password: string) => Promise<User>;
  verifyEmail: (token: string) => Promise<User>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthState | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User>();
  const [loading, setLoading] = useState(true);

  const loadUser = useCallback(async () => {
    try {
      setUser((await api.me()).user);
    } catch (error) {
      if (!(error instanceof APIError) || error.status !== 401) throw error;
      setUser(undefined);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadUser(); }, [loadUser]);
  useEffect(() => {
    const unauthorized = () => setUser(undefined);
    window.addEventListener("cronpilot:unauthorized", unauthorized);
    return () => window.removeEventListener("cronpilot:unauthorized", unauthorized);
  }, []);

  const value = useMemo<AuthState>(() => ({
    user,
    loading,
    login: async (email, password) => {
      const response = await api.login({ email, password });
      setUser(response.user);
      return response.user;
    },
    register: async (name, email, password) => (await api.register({ name, email, password })).user,
    verifyEmail: async (token) => (await api.verifyEmail(token)).user,
    logout: async () => {
      await api.logout();
      setUser(undefined);
    },
  }), [loading, user]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used inside AuthProvider");
  return value;
}

export function ProtectedRoute({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth();
  const { pick } = useLanguage();
  const location = useLocation();
  if (loading) return <div className="route-loading">{pick("正在确认登录状态……", "Checking your session…")}</div>;
  if (!user) return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  return children;
}
