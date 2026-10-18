import { useApi } from "../../shared/api/context";
import { deliveryStatuses, type DeliveryStatus } from "../../shared/api/types";
import { statusLabel } from "../../shared/format";
import { t } from "../../shared/i18n";
import { readListQuery, updateListQuery } from "../../shared/listQuery";
import { href } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { Icon } from "../../shared/ui/Icon";
import { PageHeader } from "../../shared/ui/PageHeader";
import { Pager } from "../../shared/ui/Pager";
import { ErrorState, Loading } from "../../shared/ui/States";
import { DeliveryTable } from "./DeliveryTable";

// Filters live in the URL (#/projects/{id}/deliveries?status=…) so views can
// be linked from the overview and shared.
export function DeliveriesPage({ projectId, query }: { projectId: string; query: URLSearchParams }) {
  const api = useApi();
  const rawStatus = query.get("status") ?? "";
  const status = (deliveryStatuses as readonly string[]).includes(rawStatus) ? (rawStatus as DeliveryStatus) : undefined;
  const endpointId = query.get("endpoint_id") ?? undefined;
  const { offset, limit } = readListQuery(query);

  const [state, reload] = useAsync(
    (signal) => api.listDeliveries(projectId, { limit, offset, status, endpoint_id: endpointId }, signal),
    [api, projectId, status, endpointId, offset, limit],
  );
  const [endpoints] = useAsync((signal) => api.listEndpoints(projectId, { limit: 100 }, signal), [api, projectId]);

  const base = href("projects", projectId, "deliveries");
  const update = (change: Record<string, string | undefined>) => updateListQuery(base, query, change);
  const filtered = status !== undefined || endpointId !== undefined;

  return (
    <section>
      <PageHeader
        title={t("nav.deliveries")}
        subtitle={t("deliveries.subtitle")}
        actions={
          <button type="button" className="button" onClick={reload}>
            <Icon name="refresh" size={15} />
            {t("common.refresh")}
          </button>
        }
      />
      <div className="toolbar">
        <div className="segmented" role="group" aria-label={t("deliveries.filterStatus")}>
          <button type="button" aria-pressed={status === undefined} onClick={() => update({ status: undefined })}>
            {t("common.all")}
          </button>
          {deliveryStatuses.map((s) => (
            <button key={s} type="button" aria-pressed={status === s} onClick={() => update({ status: s })}>
              {statusLabel(s)}
            </button>
          ))}
        </div>
        <label className="select-field">
          <span className="sr-only">{t("deliveries.filterEndpoint")}</span>
          <select value={endpointId ?? ""} onChange={(e) => update({ endpoint_id: e.target.value || undefined })}>
            <option value="">{t("deliveries.allEndpoints")}</option>
            {endpoints.status === "ready" &&
              endpoints.data.endpoints.map((endpoint) => (
                <option key={endpoint.id} value={endpoint.id}>
                  {endpoint.url}
                </option>
              ))}
            {endpointId && !(endpoints.status === "ready" && endpoints.data.endpoints.some((e) => e.id === endpointId)) && <option value={endpointId}>{endpointId.slice(0, 8)}</option>}
          </select>
        </label>
        {filtered && (
          <button type="button" className="button button-ghost" onClick={() => update({ status: undefined, endpoint_id: undefined })}>
            {t("common.clearFilters")}
          </button>
        )}
      </div>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" && (
        <div className="panel panel-flush">
          <DeliveryTable
            flush
            projectId={projectId}
            deliveries={state.data.deliveries}
            emptyText={filtered ? t("deliveries.noMatch") : t("deliveries.emptyHint")}
          />
          {(state.data.deliveries.length > 0 || offset > 0) && (
            <Pager
              offset={offset}
              limit={limit}
              count={state.data.deliveries.length}
              nextOffset={state.data.next_offset}
              onOffsetChange={(o) => update({ offset: String(o) })}
              onLimitChange={(l) => update({ limit: String(l) })}
            />
          )}
        </div>
      )}
    </section>
  );
}
