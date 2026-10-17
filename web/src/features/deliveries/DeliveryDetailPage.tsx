import { useState } from "react";
import { useApi } from "../../shared/api/context";
import { isTerminal } from "../../shared/api/types";
import { href, navigate } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { formatRelative, formatTime, shortId } from "../../shared/format";
import { describeError } from "../../shared/errors";
import { Empty, ErrorState, Loading } from "../../shared/ui/States";
import { HttpStatus, StatusBadge } from "../../shared/ui/StatusBadge";

export function DeliveryDetailPage({ projectId, deliveryId }: { projectId: string; deliveryId: string }) {
  const api = useApi();
  const [state, reload] = useAsync(
    async (signal) => {
      const [delivery, attempts] = await Promise.all([api.getDelivery(deliveryId, signal), api.listAttempts(deliveryId, signal)]);
      return { delivery, attempts: attempts.attempts };
    },
    [api, deliveryId],
  );

  return (
    <section>
      <p className="crumbs">
        <a href={href("projects", projectId, "deliveries")}>Deliveries</a> / <code>{deliveryId}</code>
      </p>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" && (
        <>
          <div className="section-head">
            <h1>
              Delivery <code>{shortId(state.data.delivery.id)}</code> <StatusBadge status={state.data.delivery.status} />
            </h1>
            <div className="actions">
              <button type="button" className="button" onClick={reload}>
                Refresh
              </button>
              <ReplayButton projectId={projectId} deliveryId={deliveryId} terminal={isTerminal(state.data.delivery.status)} />
            </div>
          </div>
          <dl className="facts">
            <dt>Event</dt>
            <dd>
              <a href={href("projects", projectId, "events", state.data.delivery.event_id)}>
                <code>{state.data.delivery.event_id}</code>
              </a>
            </dd>
            <dt>Endpoint</dt>
            <dd>
              <a href={href("projects", projectId, "deliveries") + `?endpoint_id=${state.data.delivery.endpoint_id}`}>
                <code>{state.data.delivery.endpoint_id}</code>
              </a>
            </dd>
            {state.data.delivery.replay_of && (
              <>
                <dt>Replay of</dt>
                <dd>
                  <a href={href("projects", projectId, "deliveries", state.data.delivery.replay_of)}>
                    <code>{state.data.delivery.replay_of}</code>
                  </a>
                </dd>
              </>
            )}
            <dt>Next attempt</dt>
            <dd>
              {state.data.delivery.next_attempt_at
                ? `${formatRelative(state.data.delivery.next_attempt_at)} (${formatTime(state.data.delivery.next_attempt_at)})`
                : "—"}
            </dd>
            <dt>Created</dt>
            <dd>{formatTime(state.data.delivery.created_at)}</dd>
          </dl>
          <h2>Attempts</h2>
          {state.data.attempts.length === 0 ? (
            <Empty title="No attempts yet">A worker will pick this delivery up when it is due.</Empty>
          ) : (
            <ol className="attempts">
              {state.data.attempts.map((a) => (
                <li key={a.id} className="attempt">
                  <div className="attempt-head">
                    <strong>#{a.attempt_number}</strong>
                    <HttpStatus code={a.status_code} />
                    <span className="muted">{a.duration_ms !== null ? `${a.duration_ms} ms` : ""}</span>
                    <span className="muted">{formatTime(a.attempted_at)}</span>
                  </div>
                  {a.error && <p className="attempt-error">{a.error}</p>}
                  {a.response_body && <pre className="code-block">{a.response_body}</pre>}
                </li>
              ))}
            </ol>
          )}
        </>
      )}
    </section>
  );
}

function ReplayButton({ projectId, deliveryId, terminal }: { projectId: string; deliveryId: string; terminal: boolean }) {
  const api = useApi();
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!terminal) {
    return (
      <button type="button" className="button" disabled title="Only succeeded or dead-lettered deliveries can be replayed">
        Replay
      </button>
    );
  }

  async function replay() {
    setBusy(true);
    setError(null);
    try {
      const created = await api.replayDelivery(deliveryId);
      navigate(href("projects", projectId, "deliveries", created.id));
    } catch (err) {
      setError(err instanceof Error ? describeError(err) : "Replay failed.");
      setConfirming(false);
    } finally {
      setBusy(false);
    }
  }

  return (
    <span className="replay">
      {confirming ? (
        <>
          <span className="muted">Send this event to the endpoint again?</span>
          <button type="button" className="button button-primary" disabled={busy} onClick={() => void replay()}>
            {busy ? "Queuing…" : "Confirm replay"}
          </button>
          <button type="button" className="button" disabled={busy} onClick={() => setConfirming(false)}>
            Cancel
          </button>
        </>
      ) : (
        <button type="button" className="button" onClick={() => setConfirming(true)}>
          Replay
        </button>
      )}
      {error && (
        <span className="field-error" role="alert">
          {error}
        </span>
      )}
    </span>
  );
}
