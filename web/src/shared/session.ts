// The API key lives in sessionStorage: it survives reloads but is dropped when
// the tab closes, and it is never written to localStorage or cookies.
const storageKey = "hooklane.apiKey";

export function loadKey(): string | null {
  try {
    return sessionStorage.getItem(storageKey);
  } catch {
    return null;
  }
}

export function saveKey(key: string): void {
  try {
    sessionStorage.setItem(storageKey, key);
  } catch {
    // Storage can be unavailable (private mode); the key then lasts for
    // this page load only.
  }
}

export function clearKey(): void {
  try {
    sessionStorage.removeItem(storageKey);
  } catch {
    // Nothing stored.
  }
}

/** Mirrors the server's format check so obvious typos fail before a request. */
export function looksLikeAPIKey(value: string): boolean {
  return /^hlk_[0-9a-f]{64}$/.test(value.trim());
}
