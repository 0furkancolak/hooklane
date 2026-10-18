import { en, type MessageKey } from "./en";
import { tr } from "./tr";

export type Lang = "en" | "tr";
export const languages: { code: Lang; label: string }[] = [
  { code: "en", label: "EN" },
  { code: "tr", label: "TR" },
];

const dictionaries: Record<Lang, Record<MessageKey, string>> = { en, tr };
export const langStorageKey = "hooklane.lang";

function isLang(value: unknown): value is Lang {
  return value === "en" || value === "tr";
}

function detect(): Lang {
  try {
    const stored = localStorage.getItem(langStorageKey);
    if (isLang(stored)) return stored;
  } catch {
    // Storage can be unavailable; fall back to the browser language.
  }
  return typeof navigator !== "undefined" && navigator.language.toLowerCase().startsWith("tr") ? "tr" : "en";
}

let current: Lang = detect();

export function currentLang(): Lang {
  return current;
}

export function setCurrentLang(next: Lang): void {
  current = next;
  try {
    localStorage.setItem(langStorageKey, next);
  } catch {
    // The choice then lasts for this tab only.
  }
}

/** BCP 47 tag for Intl formatting in the current language. */
export function locale(): string {
  return current === "tr" ? "tr-TR" : "en-US";
}

/**
 * Translates a message. It reads the current language directly so plain
 * helpers can use it too; LangProvider remounts the tree on a language
 * change, which is what makes components pick the new text up.
 */
export function t(key: MessageKey, vars?: Record<string, string | number>): string {
  const template = dictionaries[current][key] ?? en[key];
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (_, name: string) => String(vars[name] ?? `{${name}}`));
}

/** Picks "<base>_one" or "<base>_other" by count; Turkish defines the same text for both. */
export function tn(base: string, count: number, vars: Record<string, string | number> = {}): string {
  const key = `${base}_${count === 1 ? "one" : "other"}` as MessageKey;
  return t(key, { count, ...vars });
}
