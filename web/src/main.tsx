import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./app/App";
import { applyStoredTheme } from "./shared/theme";
import "./index.css";

applyStoredTheme();

const root = document.getElementById("root");
if (!root) {
  throw new Error("root element is missing");
}

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
