import { Fragment, useMemo, useState } from "react";
import { ApiClient } from "../../shared/api/client";
import { ApiContext, useApi } from "../../shared/api/context";
import { deliveryStatuses, isTerminal, type Delivery, type DeliveryStatus } from "../../shared/api/types";
import { describeError } from "../../shared/errors";
import { formatRelative, formatTime, shortId, statusLabel } from "../../shared/format";
import { languages, t, useLang } from "../../shared/i18n";
import { productName } from "../../shared/product";
import { useTheme } from "../../shared/theme";
import { useAsync } from "../../shared/useAsync";
import { Icon } from "../../shared/ui/Icon";
import { Pager } from "../../shared/ui/Pager";
import { Empty, ErrorState, Loading } from "../../shared/ui/States";
import { HttpStatus, StatusBadge } from "../../shared/ui/StatusBadge";
import { CreateEndpoint, EndpointRow, SecretNotice, type EndpointOps } from "../endpoints/EndpointsPage";

/**
 * The page a customer opens from a portal link. It talks to /v1/portal/* with
 * the link's token only; the admin API key is never involved.
 */
export function PortalPage({ token }: { token: string }) {
  const [expired, setExpired] = useState(false);
  const client = useMemo(() => new ApiClient(token, { onUnauthorized: () => setExpired(true) }), [token]);
  const [theme, toggleTheme] = useTheme();
  const { lang, setLang } = useLang();

  return (
    <ApiContext.Provider value={client}>
      <div className="portal">
        <header className="portal-bar">
          <span className="brand">{productName}</span>
          <div className="portal-bar-tools">
            <div className="segmented" role="group" aria-label={t("nav.language")}>
              {languages.map((l) => (
                <button key={l.code} type="button" aria-pressed={lang === l.code} onClick={() => setLang(l.code)}>
                  {l.label}
                </button>
              ))}
            </div>
            <button type="button" className="button button-ghost" onClick={toggleTheme} aria-label={t("nav.switchTheme")}>
              <Icon name={theme === "dark" ? "sun" : "moon"} />
            </button>
          </div>
        </header>
        <main className="portal-content">{expired ? <Expired /> : <PortalBody />}</main>
      </div>
    </ApiContext.Provider>
  );
}

function Expired() {
  return (
    <div className="state state-error" role="alert">
      <p className="state-title">{t("portal.expired.title")}</p>
      <p>{t("portal.expired.text")}</p>
    </div>
  );
}

const portalOps = (api: ApiClient): EndpointOps => ({
  update: (id, change) => api.updatePortalEndpoint(id, change),
  secret: (id) => api.getPortalEndpointSecret(id),
});

function PortalBody() {
  const api = useApi();
  const [me] = useAsync((signal) => api.portalMe(signal), [api]);
  if (me.status === "loading") return <Loading />;
  if (me.status === "error") return me.error instanceof Error && "status" in me.error && (me.error as { status: number }).status === 401 ? null : <ErrorState error={me.error} />;

  return (
    <>
      <header className="page-head">
        <div className="page-head-text">
          <h1>{t("portal.title", { customer: me.data.customer })}</h1>
          <p className="subtitle">{t("portal.subtitle", { project: me.data.project_name, date: formatTime(me.data.expires_at) })}</p>
        </div>
      </header>
      <PortalEndpoints />
      <PortalDeliveries />
    </>
  );
}

function PortalEndpoints() {
  const api = useApi();
  const [state, reload] = useAsync((signal) => api.listPortalEndpoints({ limit: 100 }, signal), [api]);
  const [created, setCreated] = useState<{ url: string; secret: string } | null>(null);

  return (
    <section>
      <h2>{t("portal.endpoints")}</h2>
      <p className="muted">{t("portal.endpoints.text")}</p>
      <div className="panel">
        <CreateEndpoint
          create={(url) => api.createPortalEndpoint(url)}
          onCreated={(endpoint) => {
            setCreated(endpoint);
            reload();
          }}
        />
      </div>
      {created && <SecretNotice url={created.url} secret={created.secret} onDismiss={() => setCreated(null)} />}
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" &&
        (state.data.endpoints.length === 0 ? (
          <Empty title={t("endpoints.none")}>{t("portal.endpoints.none")}</Empty>
        ) : (
          <ul className="rows panel panel-flush">
            {state.data.endpoints.map((endpoint) => (
              <EndpointRow key={endpoint.id} endpoint={endpoint} onChanged={reload} ops={portalOps(api)} />
            ))}
          </ul>
        ))}
    </section>
  );
}

function PortalDeliveries() {
  const api = useApi();
  const [status, setStatus] = useState<DeliveryStatus | undefined>(undefined);
  const [offset, setOffset] = useState(0);
  const [limit, setLimit] = useState(10);
  const [state, reload] = useAsync((signal) => api.listPortalDeliveries({ limit, offset, status }, signal), [api, limit, offset, status]);

  return (
    <section>
      <h2>{t("nav.deliveries")}</h2>
      <p className="muted">{t("portal.deliveries.text")}</p>
      <div className="toolbar">
        <div className="segmented" role="group" aria-label={t("deliveries.filterStatus")}>
          <button type="button" aria-pressed={status === undefined} onClick={() => (setStatus(undefined), setOffset(0))}>
            {t("common.all")}
          </button>
          {deliveryStatuses.map((s) => (
            <button key={s} type="button" aria-pressed={status === s} onClick={() => (setStatus(s), setOffset(0))}>
              {statusLabel(s)}
            </button>
          ))}
        </div>
        <button type="button" className="button" onClick={reload}>
          <Icon name="refresh" size={15} />
          {t("common.refresh")}
        </button>
      </div>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" &&
        (state.data.deliveries.length === 0 && offset === 0 ? (
          <Empty title={t("deliveries.none")}>{t("portal.deliveries.none")}</Empty>
        ) : (
          <div className="panel panel-flush">
            <div className="table-wrap table-wrap-flush">
              <table className="table">
                <thead>
                  <tr>
                    <th>{t("deliveries.col.delivery")}</th>
                    <th>{t("deliveries.col.status")}</th>
                    <th>{t("deliveries.col.attempts")}</th>
                    <th>{t("deliveries.col.response")}</th>
                    <th>{t("deliveries.col.next")}</th>
                    <th>{t("deliveries.col.created")}</th>
                  </tr>
                </thead>
                <tbody>
                  {state.data.deliveries.map((d) => (
                    <PortalDeliveryRow key={d.id} delivery={d} onReplayed={reload} />
                  ))}
                </tbody>
              </table>
            </div>
            <Pager
              offset={offset}
              limit={limit}
              count={state.data.deliveries.length}
              nextOffset={state.data.next_offset}
              onOffsetChange={setOffset}
              onLimitChange={(l) => (setLimit(l), setOffset(0))}
            />
          </div>
        ))}
    </section>
  );
}

function PortalDeliveryRow({ delivery, onReplayed }: { delivery: Delivery; onReplayed: () => void }) {
  const api = useApi();
  const [open, setOpen] = useState(false);
  const [attempts, reloadAttempts] = useAsync((signal) => (open ? api.listPortalAttempts(delivery.id, signal) : Promise.resolve(null)), [api, delivery.id, open]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function replay() {
    setBusy(true);
    setError(null);
    try {
      await api.replayPortalDelivery(delivery.id);
      onReplayed();
    } catch (err) {
      setError(err instanceof Error ? describeError(err) : t("delivery.replayFailed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Fragment>
      <tr>
        <td>
          <button type="button" className="link-button" aria-expanded={open} onClick={() => setOpen(!open)}>
            <code>{shortId(delivery.id)}</code>
            {delivery.replay_of && <span className="tag">{t("deliveries.replay")}</span>}
          </button>
        </td>
        <td>
          <StatusBadge status={delivery.status} />
        </td>
        <td>{delivery.attempt_count}</td>
        <td>{delivery.attempt_count === 0 ? <span className="muted">—</span> : <HttpStatus code={delivery.last_response_status} />}</td>
        <td>{delivery.next_attempt_at ? formatRelative(delivery.next_attempt_at) : "—"}</td>
        <td>{formatTime(delivery.created_at)}</td>
      </tr>
      {open && (
        <tr className="detail-row">
          <td colSpan={6}>
            {attempts.status === "loading" && <Loading />}
            {attempts.status === "error" && <ErrorState error={attempts.error} onRetry={reloadAttempts} />}
            {attempts.status === "ready" && attempts.data && (
              <>
                {attempts.data.attempts.length === 0 ? (
                  <p className="muted">{t("delivery.noAttemptsHint")}</p>
                ) : (
                  <ol className="attempts">
                    {attempts.data.attempts.map((a) => (
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
                {isTerminal(delivery.status) && (
                  <button type="button" className="button" disabled={busy} onClick={() => void replay()}>
                    {busy ? t("delivery.queuing") : t("delivery.replay")}
                  </button>
                )}
                {error && (
                  <p className="field-error" role="alert">
                    {error}
                  </p>
                )}
              </>
            )}
          </td>
        </tr>
      )}
    </Fragment>
  );
}
