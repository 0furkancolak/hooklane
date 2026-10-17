import type { ReactNode } from "react";
import { render } from "@testing-library/react";
import { ApiClient } from "../shared/api/client";
import { ApiContext } from "../shared/api/context";

export const testKey = "hlk_" + "a".repeat(64);

type Handler = (init: { method: string; url: URL; body: unknown }) => { status?: number; body?: unknown } | undefined;

export type Call = { method: string; path: string; search: string; body: unknown; auth: string | null };

/** A fetch double that routes "METHOD /path" to handlers and records calls. */
export function fakeFetch(routes: Record<string, Handler>) {
  const calls: Call[] = [];
  const impl = async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = new URL(String(input), "http://hooklane.test");
    const method = init.method ?? "GET";
    const headers = new Headers(init.headers);
    const body = typeof init.body === "string" ? JSON.parse(init.body) : undefined;
    calls.push({ method, path: url.pathname, search: url.search, body, auth: headers.get("Authorization") });
    const handler = routes[`${method} ${url.pathname}`];
    const result = handler?.({ method, url, body }) ?? { status: 404, body: { error: { code: "not_found", message: "not found" } } };
    return new Response(result.body === undefined ? null : JSON.stringify(result.body), {
      status: result.status ?? 200,
      headers: { "Content-Type": "application/json" },
    });
  };
  return { fetch: impl as typeof fetch, calls };
}

export function renderWithApi(ui: ReactNode, fetchImpl: typeof fetch, onUnauthorized?: () => void) {
  const client = new ApiClient(testKey, { fetch: fetchImpl, onUnauthorized });
  return render(<ApiContext.Provider value={client}>{ui}</ApiContext.Provider>);
}

export const ids = {
  project: "11111111-1111-4111-8111-111111111111",
  endpoint: "22222222-2222-4222-8222-222222222222",
  event: "33333333-3333-4333-8333-333333333333",
  delivery: "44444444-4444-4444-8444-444444444444",
  replay: "55555555-5555-4555-8555-555555555555",
};

export function delivery(overrides: Record<string, unknown> = {}) {
  return {
    id: ids.delivery,
    event_id: ids.event,
    endpoint_id: ids.endpoint,
    status: "dead_letter",
    attempt_count: 2,
    next_attempt_at: null,
    last_response_status: 503,
    last_error: "endpoint responded with status 503",
    replay_of: null,
    created_at: "2026-10-17T10:00:00Z",
    updated_at: "2026-10-17T10:05:00Z",
    ...overrides,
  };
}

export function page<K extends string>(key: K, items: unknown[], next: number | null = null) {
  return { [key]: items, limit: 25, offset: 0, next_offset: next };
}
