import { Navigate, Route, Routes } from "react-router-dom";
import { ConsoleLayout } from "./layouts/ConsoleLayout";
import { AuthPage } from "./pages/AuthPage";
import { DeliveryPage } from "./pages/DeliveryPage";
import { LandingPage } from "./pages/LandingPage";
import { OverviewPage } from "./pages/OverviewPage";
import { RunsPage } from "./pages/RunsPage";
import { SystemPage } from "./pages/SystemPage";
import { TasksPage } from "./pages/TasksPage";
import { StateProvider } from "./state";
import { AuthProvider, ProtectedRoute } from "./auth";
import { VerifyEmailPage } from "./pages/VerifyEmailPage";

export default function App() {
  return (
    <AuthProvider><Routes>
      <Route path="/" element={<LandingPage />} />
      <Route path="/login" element={<AuthPage mode="login" />} />
      <Route path="/register" element={<AuthPage mode="register" />} />
      <Route path="/verify-email" element={<VerifyEmailPage />} />
      <Route path="/app" element={<ProtectedRoute><StateProvider><ConsoleLayout /></StateProvider></ProtectedRoute>}>
        <Route index element={<Navigate to="overview" replace />} />
        <Route path="overview" element={<OverviewPage />} />
        <Route path="tasks" element={<TasksPage />} />
        <Route path="runs" element={<RunsPage />} />
        <Route path="delivery" element={<DeliveryPage />} />
        <Route path="system" element={<SystemPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes></AuthProvider>
  );
}
