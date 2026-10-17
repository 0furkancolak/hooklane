import type {
  Attempt,
  CreatedEndpoint,
  Delivery,
  DeliveryStats,
  DeliveryStatus,
  Endpoint,
  HooklaneEvent,
  Page,
  Project,
} from "./types";

/** An error response from the API: `{"error": {"code", "message"}}`. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

export type PageParams = { limit?: number; offset?: number };

type Fetch = typeof fetch;

/**
 * Typed wrapper around the management API. It only ever sends the key in the
 * Authorization header and never logs it. onUnauthorized lets the app drop a
 * revoked or wrong key and return to the sign-in screen.
 */
export class ApiClient {
  private readonly key: string;
  private readonly fetchImpl: Fetch;
  private readonly onUnauthorized?: () => void;

  constructor(key: string, options: { fetch?: Fetch; onUnauthorized?: () => void } = {}) {
    this.key = key;
    this.fetchImpl = options.fetch ?? ((input, init) => fetch(input, init));
    this.onUnauthorized = options.onUnauthorized;
  }

  listProjects(page: PageParams = {}, signal?: AbortSignal) {
    return this.get<Page<"projects", Project>>(`/v1/projects${query(page)}`, signal);
  }

  getProject(id: string, signal?: AbortSignal) {
    return this.get<Project>(`/v1/projects/${seg(id)}`, signal);
  }

  listEndpoints(projectId: string, params: PageParams & { enabled?: boolean } = {}, signal?: AbortSignal) {
    return this.get<Page<"endpoints", Endpoint>>(`/v1/projects/${seg(projectId)}/endpoints${query(params)}`, signal);
  }

  createEndpoint(projectId: string, url: string) {
    return this.send<CreatedEndpoint>("POST", `/v1/projects/${seg(projectId)}/endpoints`, { url });
  }

  updateEndpoint(id: string, change: { url?: string; enabled?: boolean }) {
    return this.send<Endpoint>("PATCH", `/v1/endpoints/${seg(id)}`, change);
  }

  getEndpointSecret(id: string) {
    return this.get<{ secret: string }>(`/v1/endpoints/${seg(id)}/secret`);
  }

  listEvents(projectId: string, params: PageParams & { event_type?: string } = {}, signal?: AbortSignal) {
    return this.get<Page<"events", HooklaneEvent>>(`/v1/projects/${seg(projectId)}/events${query(params)}`, signal);
  }

  getEvent(id: string, signal?: AbortSignal) {
    return this.get<HooklaneEvent>(`/v1/events/${seg(id)}`, signal);
  }

  listEventDeliveries(eventId: string, signal?: AbortSignal) {
    return this.get<{ deliveries: Delivery[] }>(`/v1/events/${seg(eventId)}/deliveries`, signal);
  }

  listDeliveries(
    projectId: string,
    params: PageParams & { status?: DeliveryStatus; endpoint_id?: string } = {},
    signal?: AbortSignal,
  ) {
    return this.get<Page<"deliveries", Delivery>>(`/v1/projects/${seg(projectId)}/deliveries${query(params)}`, signal);
  }

  getDeliveryStats(projectId: string, signal?: AbortSignal) {
    return this.get<DeliveryStats>(`/v1/projects/${seg(projectId)}/delivery-stats`, signal);
  }

  getDelivery(id: string, signal?: AbortSignal) {
    return this.get<Delivery>(`/v1/deliveries/${seg(id)}`, signal);
  }

  listAttempts(id: string, signal?: AbortSignal) {
    return this.get<{ attempts: Attempt[] }>(`/v1/deliveries/${seg(id)}/attempts`, signal);
  }

  replayDelivery(id: string) {
    return this.send<Delivery>("POST", `/v1/deliveries/${seg(id)}/replay`);
  }

  private get<T>(path: string, signal?: AbortSignal): Promise<T> {
    return this.request<T>("GET", path, undefined, signal);
  }

  private send<T>(method: string, path: string, body?: unknown): Promise<T> {
    return this.request<T>(method, path, body);
  }

  private async request<T>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
    const headers: Record<string, string> = { Authorization: `Bearer ${this.key}` };
    if (body !== undefined) {
      headers["Content-Type"] = "application/json";
    }
    let response: Response;
    try {
      response = await this.fetchImpl(path, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
        signal,
        cache: "no-store",
      });
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") {
        throw err;
      }
      throw new ApiError(0, "network_error", "Could not reach the Hooklane API");
    }
    const text = await response.text();
    const data: unknown = text ? safeJSON(text) : undefined;
    if (!response.ok) {
      const detail = (data as { error?: { code?: string; message?: string } } | undefined)?.error;
      const error = new ApiError(response.status, detail?.code ?? "http_error", detail?.message ?? `Request failed (${response.status})`);
      if (response.status === 401) {
        this.onUnauthorized?.();
      }
      throw error;
    }
    return data as T;
  }
}

function safeJSON(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

function seg(value: string): string {
  return encodeURIComponent(value);
}

function query(params: Record<string, string | number | boolean | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== "") {
      search.set(key, String(value));
    }
  }
  const encoded = search.toString();
  return encoded ? `?${encoded}` : "";
}
