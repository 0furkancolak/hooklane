import { useApi } from "../../shared/api/context";
import { deliveryStatuses, type DeliveryStatus } from "../../shared/api/types";
import { href, navigate } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { statusLabel } from "../../shared/format";
import { ErrorState, Loading } from "../../shared/ui/States";
import { Pager } from "../../shared/ui/Pager";
import { DeliveryTable } from "./DeliveryTable";

const pageSize = 25;

// Filters live in the URL (#/projects/{id}/deliveries?status=…) so views can
// be linked from the overview and shared.
export function DeliveriesPage({ projectId, query }: { projectId: string; query: URLSearchParams }) {
  const api = useApi();
  const rawStatus = query.get("status") ?? "";
  const status = (deliveryStatuses as readonly string[]).includes(rawStatus) ? (rawStatus as DeliveryStatus) : undefined;
  const endpointId = query.get("endpoint_id") ?? undefined;
  const offset = Math.max(0, Number(query.get("offset") ?? 0) || 0);

  const [state, reload] = useAsync(
    (signal) => api.listDeliveries(projectId, { limit: pageSize, offset, status, endpoint_id: endpointId }, signal),
    [api, projectId, status, endpointId, offset],
  );

  function update(change: Record<string, string | undefined>) {
    const next = new URLSearchParams(query);
    for (const [key, value] of Object.entries(change)) {
      if (value === undefined || value === "") next.delete(key);
      else next.set(key, value);
    }
    if (!("offset" in change)) next.delete("offset");
    const search = next.toString();
    navigate(href("projects", projectId, "deliveries") + (search ? `?${search}` : ""));
  }

  return (
    <section>
      <div className="section-head">
        <h1>Deliveries</h1>
        <button type="button" className="button" onClick={reload}>
          Refresh
        </button>
      </div>
      <div className="filters" role="group" aria-label="Filter by status">
        <button type="button" className={`chip ${status === undefined ? "chip-active" : ""}`} onClick={() => update({ status: undefined })}>
          all
        </button>
        {deliveryStatuses.map((s) => (
          <button key={s} type="button" className={`chip ${status === s ? "chip-active" : ""}`} onClick={() => update({ status: s })}>
            {statusLabel(s)}
          </button>
        ))}
        {endpointId && (
          <button type="button" className="chip chip-active" onClick={() => update({ endpoint_id: undefined })} title="Clear endpoint filter">
            endpoint {endpointId.slice(0, 8)} ×
          </button>
        )}
      </div>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" && (
        <>
          <DeliveryTable
            projectId={projectId}
            deliveries={state.data.deliveries}
            emptyText={status || endpointId ? "Nothing matches these filters." : "Deliveries appear here once events arrive for enabled endpoints."}
          />
          <Pager offset={offset} limit={pageSize} nextOffset={state.data.next_offset} onChange={(o) => update({ offset: String(o) })} />
        </>
      )}
    </section>
  );
}
