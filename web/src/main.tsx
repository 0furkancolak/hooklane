import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@fontsource-variable/inter";
import App from "./app/App";
import { LangProvider } from "./shared/i18n";
import { applyStoredTheme } from "./shared/theme";
import "./index.css";

applyStoredTheme();

const root = document.getElementById("root");
if (!root) {
  throw new Error("root element is missing");
}

createRoot(root).render(
  <StrictMode>
    <LangProvider>
      <App />
    </LangProvider>
  </StrictMode>,
);
