import { useState, type FormEvent } from "react";
import { useApi } from "../../shared/api/context";
import { href } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { formatTime, shortId } from "../../shared/format";
import { Empty, ErrorState, Loading } from "../../shared/ui/States";
import { Pager } from "../../shared/ui/Pager";

const pageSize = 25;

export function EventsPage({ projectId }: { projectId: string }) {
  const api = useApi();
  const [offset, setOffset] = useState(0);
  const [typeInput, setTypeInput] = useState("");
  const [eventType, setEventType] = useState("");
  const [state, reload] = useAsync(
    (signal) => api.listEvents(projectId, { limit: pageSize, offset, event_type: eventType || undefined }, signal),
    [api, projectId, offset, eventType],
  );

  function applyFilter(event: FormEvent) {
    event.preventDefault();
    setOffset(0);
    setEventType(typeInput.trim());
  }

  return (
    <section>
      <div className="section-head">
        <h1>Events</h1>
        <button type="button" className="button" onClick={reload}>
          Refresh
        </button>
      </div>
      <form className="inline-form" onSubmit={applyFilter}>
        <label htmlFor="event-type" className="sr-only">
          Event type
        </label>
        <input id="event-type" placeholder="Filter by event type, e.g. order.paid" value={typeInput} onChange={(e) => setTypeInput(e.target.value)} />
        <button type="submit" className="button">
          Filter
        </button>
      </form>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" &&
        (state.data.events.length === 0 ? (
          <Empty title={eventType ? `No "${eventType}" events` : "No events yet"}>
            {!eventType && "Send one with POST /v1/projects/{id}/events."}
          </Empty>
        ) : (
          <>
            <div className="table-wrap">
      <table className="table">
              <thead>
                <tr>
                  <th>Event</th>
                  <th>Type</th>
                  <th>Idempotency key</th>
                  <th>Received</th>
                </tr>
              </thead>
              <tbody>
                {state.data.events.map((event) => (
                  <tr key={event.id}>
                    <td>
                      <a href={href("projects", projectId, "events", event.id)}>
                        <code>{shortId(event.id)}</code>
                      </a>
                    </td>
                    <td>
                      <code>{event.event_type}</code>
                    </td>
                    <td className="muted">{event.idempotency_key ?? "—"}</td>
                    <td>{formatTime(event.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
      </div>
            <Pager offset={offset} limit={pageSize} nextOffset={state.data.next_offset} onChange={setOffset} />
          </>
        ))}
    </section>
  );
}
