// Response shapes of the Hooklane management API (see README "API overview").

export type Project = {
  id: string;
  organization_id: string;
  name: string;
  created_at: string;
};

export type Endpoint = {
  id: string;
  project_id: string;
  url: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
};

export type IngestedEvent = HooklaneEvent & { delivery_count: number };

/** A webhook captured by the development-only built-in receiver. */
export type SinkRequest = {
  received_at: string;
  method: string;
  headers: Record<string, string>;
  body: string;
  truncated: boolean;
  status: number;
};

export type CreatedEndpoint = Endpoint & { secret: string };

export type APIKey = {
  id: string;
  project_id: string;
  name: string;
  prefix: string;
  revoked: boolean;
  created_at: string;
};

export type CreatedAPIKey = APIKey & { raw_key: string };

export type HooklaneEvent = {
  id: string;
  project_id: string;
  event_type: string;
  payload: unknown;
  idempotency_key: string | null;
  created_at: string;
};

export const deliveryStatuses = ["pending", "processing", "succeeded", "dead_letter"] as const;
export type DeliveryStatus = (typeof deliveryStatuses)[number];

export type Delivery = {
  id: string;
  event_id: string;
  endpoint_id: string;
  status: DeliveryStatus;
  attempt_count: number;
  next_attempt_at: string | null;
  last_response_status: number | null;
  last_error: string | null;
  replay_of: string | null;
  created_at: string;
  updated_at: string;
};

export type Attempt = {
  id: string;
  attempt_number: number;
  status_code: number | null;
  response_body: string | null;
  error: string | null;
  duration_ms: number | null;
  attempted_at: string;
};

export type DeliveryStats = {
  pending: number;
  processing: number;
  succeeded: number;
  dead_letter: number;
  oldest_due_at: string | null;
};

export type Page<K extends string, T> = { [key in K]: T[] } & {
  limit: number;
  offset: number;
  next_offset: number | null;
};

export function isTerminal(status: DeliveryStatus): boolean {
  return status === "succeeded" || status === "dead_letter";
}
