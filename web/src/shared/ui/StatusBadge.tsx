import type { DeliveryStatus } from "../api/types";
import { statusLabel } from "../format";

export function StatusBadge({ status }: { status: DeliveryStatus }) {
  return <span className={`badge badge-${status}`}>{statusLabel(status)}</span>;
}

export function EnabledBadge({ enabled }: { enabled: boolean }) {
  return <span className={`badge ${enabled ? "badge-succeeded" : "badge-muted"}`}>{enabled ? "enabled" : "disabled"}</span>;
}

export function HttpStatus({ code }: { code: number | null }) {
  if (code === null) return <span className="muted">no response</span>;
  const tone = code >= 200 && code < 300 ? "ok" : code >= 500 || code === 429 || code === 408 ? "warn" : "bad";
  return <code className={`http http-${tone}`}>{code}</code>;
}
