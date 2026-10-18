import { useApi } from "../../shared/api/context";
import { formatTime } from "../../shared/format";
import { t } from "../../shared/i18n";
import { href } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { Icon } from "../../shared/ui/Icon";
import { PageHeader } from "../../shared/ui/PageHeader";
import { ErrorState, Loading } from "../../shared/ui/States";
import { DeliveryTable } from "../deliveries/DeliveryTable";

export function EventDetailPage({ projectId, eventId }: { projectId: string; eventId: string }) {
  const api = useApi();
  const [state, reload] = useAsync(
    async (signal) => {
      const [event, deliveries] = await Promise.all([api.getEvent(eventId, signal), api.listEventDeliveries(eventId, signal)]);
      return { event, deliveries: deliveries.deliveries };
    },
    [api, eventId],
  );

  return (
    <section>
      <p className="crumbs">
        <a href={href("projects", projectId, "events")}>{t("nav.events")}</a> / <code>{eventId}</code>
      </p>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" && (
        <>
          <PageHeader
            title={<code>{state.data.event.event_type}</code>}
            actions={
              <button type="button" className="button" onClick={reload}>
                <Icon name="refresh" size={15} />
                {t("common.refresh")}
              </button>
            }
          />
          <div className="panel">
            <dl className="facts">
              <dt>{t("events.col.received")}</dt>
              <dd>{formatTime(state.data.event.created_at)}</dd>
              <dt>{t("events.col.idempotency")}</dt>
              <dd>{state.data.event.idempotency_key ?? "—"}</dd>
            </dl>
          </div>
          <h2>{t("events.payload")}</h2>
          <pre className="code-block">{JSON.stringify(state.data.event.payload, null, 2)}</pre>
          <h2>{t("nav.deliveries")}</h2>
          <DeliveryTable projectId={projectId} deliveries={state.data.deliveries} emptyText={t("events.noDeliveries")} />
        </>
      )}
    </section>
  );
}
