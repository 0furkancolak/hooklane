import { useState } from "react";
import { useApi } from "../../shared/api/context";
import { isTerminal } from "../../shared/api/types";
import { href, navigate } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { formatRelative, formatTime, shortId } from "../../shared/format";
import { describeError } from "../../shared/errors";
import { t } from "../../shared/i18n";
import { Icon } from "../../shared/ui/Icon";
import { PageHeader } from "../../shared/ui/PageHeader";
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
        <a href={href("projects", projectId, "deliveries")}>{t("nav.deliveries")}</a> / <code>{deliveryId}</code>
      </p>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" && (
        <>
          <PageHeader
            title={
              <>
                {t("crumb.delivery")} <code>{shortId(state.data.delivery.id)}</code> <StatusBadge status={state.data.delivery.status} />
              </>
            }
            actions={
              <>
                <button type="button" className="button" onClick={reload}>
                  <Icon name="refresh" size={15} />
                  {t("common.refresh")}
                </button>
                <ReplayButton projectId={projectId} deliveryId={deliveryId} terminal={isTerminal(state.data.delivery.status)} />
              </>
            }
          />
          <div className="panel">
          <dl className="facts">
            <dt>{t("crumb.event")}</dt>
            <dd>
              <a href={href("projects", projectId, "events", state.data.delivery.event_id)}>
                <code>{state.data.delivery.event_id}</code>
              </a>
            </dd>
            <dt>{t("delivery.endpoint")}</dt>
            <dd>
              <a href={href("projects", projectId, "deliveries") + `?endpoint_id=${state.data.delivery.endpoint_id}`}>
                <code>{state.data.delivery.endpoint_id}</code>
              </a>
            </dd>
            {state.data.delivery.replay_of && (
              <>
                <dt>{t("delivery.replayOf")}</dt>
                <dd>
                  <a href={href("projects", projectId, "deliveries", state.data.delivery.replay_of)}>
                    <code>{state.data.delivery.replay_of}</code>
                  </a>
                </dd>
              </>
            )}
            <dt>{t("deliveries.col.next")}</dt>
            <dd>
              {state.data.delivery.next_attempt_at
                ? `${formatRelative(state.data.delivery.next_attempt_at)} (${formatTime(state.data.delivery.next_attempt_at)})`
                : "—"}
            </dd>
            <dt>{t("deliveries.col.created")}</dt>
            <dd>{formatTime(state.data.delivery.created_at)}</dd>
          </dl>
          </div>
          <h2>{t("deliveries.col.attempts")}</h2>
          {state.data.attempts.length === 0 ? (
            <Empty title={t("delivery.noAttempts")}>{t("delivery.noAttemptsHint")}</Empty>
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
      <button type="button" className="button" disabled title={t("delivery.replayDisabled")}>
        {t("delivery.replay")}
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
      setError(err instanceof Error ? describeError(err) : t("delivery.replayFailed"));
      setConfirming(false);
    } finally {
      setBusy(false);
    }
  }

  return (
    <span className="replay">
      {confirming ? (
        <>
          <span className="muted">{t("delivery.replayConfirm")}</span>
          <button type="button" className="button button-primary" disabled={busy} onClick={() => void replay()}>
            {busy ? t("delivery.queuing") : t("delivery.confirmReplay")}
          </button>
          <button type="button" className="button" disabled={busy} onClick={() => setConfirming(false)}>
            {t("common.cancel")}
          </button>
        </>
      ) : (
        <button type="button" className="button" onClick={() => setConfirming(true)}>
          {t("delivery.replay")}
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
