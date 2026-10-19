import type {
  Account,
  AccountSession,
  APIKey,
  Attempt,
  CreatedAPIKey,
  CreatedEndpoint,
  Delivery,
  DeliveryStats,
  DeliveryStatus,
  Endpoint,
  HooklaneEvent,
  IngestedEvent,
  Page,
  PortalLink,
  PortalMe,
  Project,
  Scope,
  SinkRequest,
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

  /** True when signed in with an account (a session token) rather than an API key. */
  get isSession(): boolean {
    return this.key.startsWith("hls_");
  }

  me(signal?: AbortSignal) {
    return this.get<Account>("/v1/auth/me", signal);
  }

  signOutSession() {
    return this.send<void>("POST", "/v1/auth/signout");
  }

  createOrganization(name: string) {
    return this.send<Account>("POST", "/v1/auth/organizations", { name });
  }

  switchOrganization(organizationId: string) {
    return this.send<Account>("POST", "/v1/auth/switch-organization", { organization_id: organizationId });
  }

  listProjects(page: PageParams = {}, signal?: AbortSignal) {
    return this.get<Page<"projects", Project>>(`/v1/projects${query(page)}`, signal);
  }

  createProject(name: string) {
    return this.send<Project>("POST", "/v1/projects", { name });
  }

  getProject(id: string, signal?: AbortSignal) {
    return this.get<Project>(`/v1/projects/${seg(id)}`, signal);
  }

  listAPIKeys(projectId: string, signal?: AbortSignal) {
    return this.get<{ api_keys: APIKey[] }>(`/v1/projects/${seg(projectId)}/api-keys`, signal);
  }

  createAPIKey(projectId: string, name: string, scopes: Scope[] = ["admin"]) {
    return this.send<CreatedAPIKey>("POST", `/v1/projects/${seg(projectId)}/api-keys`, { name, scopes });
  }

  revokeAPIKey(id: string) {
    return this.send<APIKey>("POST", `/v1/api-keys/${seg(id)}/revoke`);
  }

  listEndpoints(projectId: string, params: PageParams & { enabled?: boolean; customer?: string } = {}, signal?: AbortSignal) {
    return this.get<Page<"endpoints", Endpoint>>(`/v1/projects/${seg(projectId)}/endpoints${query(params)}`, signal);
  }

  createEndpoint(projectId: string, url: string, customer?: string) {
    return this.send<CreatedEndpoint>("POST", `/v1/projects/${seg(projectId)}/endpoints`, customer ? { url, customer } : { url });
  }

  createPortalLink(projectId: string, customer: string, ttlSeconds?: number) {
    return this.send<PortalLink>("POST", `/v1/projects/${seg(projectId)}/portal-links`, ttlSeconds ? { customer, ttl_seconds: ttlSeconds } : { customer });
  }

  // Portal routes: the client must have been built with a portal token (hlp_…).
  portalMe(signal?: AbortSignal) {
    return this.get<PortalMe>("/v1/portal/me", signal);
  }

  listPortalEndpoints(params: PageParams = {}, signal?: AbortSignal) {
    return this.get<Page<"endpoints", Endpoint>>(`/v1/portal/endpoints${query(params)}`, signal);
  }

  createPortalEndpoint(url: string) {
    return this.send<CreatedEndpoint>("POST", "/v1/portal/endpoints", { url });
  }

  updatePortalEndpoint(id: string, change: { url?: string; enabled?: boolean }) {
    return this.send<Endpoint>("PATCH", `/v1/portal/endpoints/${seg(id)}`, change);
  }

  getPortalEndpointSecret(id: string) {
    return this.get<{ secret: string }>(`/v1/portal/endpoints/${seg(id)}/secret`);
  }

  listPortalDeliveries(params: PageParams & { status?: DeliveryStatus } = {}, signal?: AbortSignal) {
    return this.get<Page<"deliveries", Delivery>>(`/v1/portal/deliveries${query(params)}`, signal);
  }

  listPortalAttempts(id: string, signal?: AbortSignal) {
    return this.get<{ attempts: Attempt[] }>(`/v1/portal/deliveries/${seg(id)}/attempts`, signal);
  }

  replayPortalDelivery(id: string) {
    return this.send<Delivery>("POST", `/v1/portal/deliveries/${seg(id)}/replay`);
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

  ingestEvent(projectId: string, event: { event_type: string; payload: unknown }) {
    return this.send<IngestedEvent>("POST", `/v1/projects/${seg(projectId)}/events`, event);
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

/**
 * Calls that run before sign-in, so they carry no API key. The development
 * routes exist only when the API runs with APP_ENV=development.
 */
export const publicApi = {
  async ready(signal?: AbortSignal): Promise<boolean> {
    try {
      const response = await fetch("/readyz", { cache: "no-store", signal });
      return response.ok;
    } catch {
      return false;
    }
  },

  async developmentMode(signal?: AbortSignal): Promise<boolean> {
    return (await publicApi.developmentInfo(signal)) !== null;
  },

  /** The development status, or null when the API is not in development mode. */
  async developmentInfo(signal?: AbortSignal): Promise<{ sinkUrl: string } | null> {
    try {
      const response = await fetch("/v1/dev", { cache: "no-store", signal });
      const data = response.ok ? (safeJSON(await response.text()) as { development?: boolean; sink_url?: string } | undefined) : undefined;
      return data?.development === true ? { sinkUrl: data.sink_url ?? "" } : null;
    } catch {
      return null;
    }
  },

  async sinkRequests(token: string, signal?: AbortSignal): Promise<SinkRequest[]> {
    try {
      const response = await fetch(`/v1/dev/sink/${encodeURIComponent(token)}`, { cache: "no-store", signal });
      const data = response.ok ? (safeJSON(await response.text()) as { requests?: SinkRequest[] } | undefined) : undefined;
      return data?.requests ?? [];
    } catch {
      return [];
    }
  },

  signUp(email: string, password: string, organizationName: string): Promise<AccountSession> {
    return postJSON<AccountSession>("/v1/auth/signup", { email, password, organization_name: organizationName });
  },

  signIn(email: string, password: string): Promise<AccountSession> {
    return postJSON<AccountSession>("/v1/auth/signin", { email, password });
  },

  async issueDevelopmentKey(): Promise<CreatedAPIKey> {
    let response: Response;
    try {
      response = await fetch("/v1/dev/api-keys", { method: "POST", cache: "no-store" });
    } catch {
      throw new ApiError(0, "network_error", "Could not reach the Hooklane API");
    }
    const data = safeJSON(await response.text());
    if (!response.ok) {
      const detail = (data as { error?: { code?: string; message?: string } } | undefined)?.error;
      throw new ApiError(response.status, detail?.code ?? "http_error", detail?.message ?? `Request failed (${response.status})`);
    }
    return data as CreatedAPIKey;
  },
};

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

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  let response: Response;
  try {
    response = await fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      cache: "no-store",
    });
  } catch {
    throw new ApiError(0, "network_error", "Could not reach the Hooklane API");
  }
  const data = safeJSON(await response.text());
  if (!response.ok) {
    const detail = (data as { error?: { code?: string; message?: string } } | undefined)?.error;
    throw new ApiError(response.status, detail?.code ?? "http_error", detail?.message ?? `Request failed (${response.status})`);
  }
  return data as T;
}
