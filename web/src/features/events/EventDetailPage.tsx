import { useApi } from "../../shared/api/context";
import { href } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { formatTime } from "../../shared/format";
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
        <a href={href("projects", projectId, "events")}>Events</a> / <code>{eventId}</code>
      </p>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" && (
        <>
          <div className="section-head">
            <h1>
              <code>{state.data.event.event_type}</code>
            </h1>
            <button type="button" className="button" onClick={reload}>
              Refresh
            </button>
          </div>
          <dl className="facts">
            <dt>Received</dt>
            <dd>{formatTime(state.data.event.created_at)}</dd>
            <dt>Idempotency key</dt>
            <dd>{state.data.event.idempotency_key ?? "—"}</dd>
          </dl>
          <h2>Payload</h2>
          <pre className="code-block">{JSON.stringify(state.data.event.payload, null, 2)}</pre>
          <h2>Deliveries</h2>
          <DeliveryTable projectId={projectId} deliveries={state.data.deliveries} emptyText="This event had no enabled endpoints when it arrived." />
        </>
      )}
    </section>
  );
}
