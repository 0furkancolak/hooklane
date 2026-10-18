import { useEffect, useState } from "react";
import { publicApi } from "../../shared/api/client";

export type Receiver = { loading: boolean; sinkUrl: string | null; token: string };

const tokenKey = "hooklane.playground.token";

function randomToken(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(8));
  return "pg-" + Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

function loadToken(): string {
  try {
    const stored = localStorage.getItem(tokenKey);
    if (stored) return stored;
    const created = randomToken();
    localStorage.setItem(tokenKey, created);
    return created;
  } catch {
    return randomToken();
  }
}

/**
 * Finds out whether the API offers the built-in development receiver and
 * holds the token that names this browser's inbox on it.
 */
export function useReceiver(): Receiver {
  const [state, setState] = useState<Receiver>(() => ({ loading: true, sinkUrl: null, token: loadToken() }));
  useEffect(() => {
    const controller = new AbortController();
    void publicApi.developmentInfo(controller.signal).then((info) => {
      if (!controller.signal.aborted) {
        setState((current) => ({ ...current, loading: false, sinkUrl: info?.sinkUrl || null }));
      }
    });
    return () => controller.abort();
  }, []);
  return state;
}
