import { useCallback, useState } from "react";

export type Theme = "light" | "dark";

const storageKey = "hooklane.theme";

function stored(): Theme | null {
  try {
    const value = localStorage.getItem(storageKey);
    return value === "light" || value === "dark" ? value : null;
  } catch {
    return null;
  }
}

function system(): Theme {
  return typeof matchMedia === "function" && matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
}

function apply(theme: Theme) {
  document.documentElement.dataset.theme = theme;
}

// Call once before the first render so the page never flashes the wrong theme.
export function applyStoredTheme() {
  apply(stored() ?? system());
}

export function useTheme(): [Theme, () => void] {
  const [theme, setTheme] = useState<Theme>(() => stored() ?? system());
  const toggle = useCallback(() => {
    const next: Theme = theme === "dark" ? "light" : "dark";
    apply(next);
    try {
      localStorage.setItem(storageKey, next);
    } catch {
      // Private windows may refuse storage; the choice then lasts for this tab only.
    }
    setTheme(next);
  }, [theme]);
  return [theme, toggle];
}
