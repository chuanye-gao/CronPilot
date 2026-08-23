import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { HashRouter } from "react-router-dom";
import App from "./App";
import { LanguageProvider } from "./i18n";
import "./styles/base.css";
import "./styles/public.css";
import "./styles/console.css";
import "./styles/components.css";
import "./styles/readability.css";
import "./styles/assistant.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <HashRouter>
      <LanguageProvider><App /></LanguageProvider>
    </HashRouter>
  </StrictMode>,
);
