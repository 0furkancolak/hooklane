import { useCallback, useEffect, useRef, useState } from "react";

export type AsyncState<T> =
  | { status: "loading" }
  | { status: "error"; error: Error }
  | { status: "ready"; data: T };

/**
 * Runs load whenever deps change, aborting the previous request, and exposes
 * reload for retries and refreshes after mutations. A reload keeps showing
 * the current data until the new response arrives; a change of deps (another
 * project, another filter) shows the loading state instead of stale data.
 */
export function useAsync<T>(load: (signal: AbortSignal) => Promise<T>, deps: unknown[]): [AsyncState<T>, () => void] {
  const [state, setState] = useState<AsyncState<T>>({ status: "loading" });
  const [nonce, setNonce] = useState(0);
  const lastNonce = useRef(nonce);

  useEffect(() => {
    const controller = new AbortController();
    const isReload = lastNonce.current !== nonce;
    lastNonce.current = nonce;
    setState((current) => (isReload && current.status === "ready" ? current : { status: "loading" }));
    load(controller.signal).then(
      (data) => {
        if (!controller.signal.aborted) setState({ status: "ready", data });
      },
      (error: unknown) => {
        if (!controller.signal.aborted) {
          setState({ status: "error", error: error instanceof Error ? error : new Error(String(error)) });
        }
      },
    );
    return () => controller.abort();
    // load is recreated every render; deps describe what it reads.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, nonce]);

  const reload = useCallback(() => setNonce((n) => n + 1), []);
  return [state, reload];
}
