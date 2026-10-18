import { useState, type FormEvent } from "react";
import { useApi } from "../../shared/api/context";
import type { Endpoint } from "../../shared/api/types";
import { href } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { formatTime } from "../../shared/format";
import { describeError } from "../../shared/errors";
import { t } from "../../shared/i18n";
import { readListQuery, updateListQuery } from "../../shared/listQuery";
import { Empty, ErrorState, Loading } from "../../shared/ui/States";
import { EnabledBadge } from "../../shared/ui/StatusBadge";
import { Icon } from "../../shared/ui/Icon";
import { PageHeader } from "../../shared/ui/PageHeader";
import { Pager } from "../../shared/ui/Pager";
import { CopyButton } from "../../shared/ui/CopyButton";
import { PortalLinkPanel } from "./PortalLinkPanel";

type EnabledFilter = "all" | "enabled" | "disabled";

export function EndpointsPage({ projectId, query }: { projectId: string; query: URLSearchParams }) {
  const api = useApi();
  const { offset, limit } = readListQuery(query);
  const rawFilter = query.get("enabled");
  const filter: EnabledFilter = rawFilter === "true" ? "enabled" : rawFilter === "false" ? "disabled" : "all";
  const enabled = filter === "all" ? undefined : filter === "enabled";
  const customer = query.get("customer") ?? "";
  const [customerInput, setCustomerInput] = useState(customer);
  const [state, reload] = useAsync((signal) => api.listEndpoints(projectId, { limit, offset, enabled, customer: customer || undefined }, signal), [api, projectId, limit, offset, enabled, customer]);
  const [created, setCreated] = useState<{ url: string; secret: string } | null>(null);

  const base = href("projects", projectId, "endpoints");
  const update = (change: Record<string, string | undefined>) => updateListQuery(base, query, change);
  const filters: { value: EnabledFilter; label: string; param: string | undefined }[] = [
    { value: "all", label: t("common.all"), param: undefined },
    { value: "enabled", label: t("badge.enabled"), param: "true" },
    { value: "disabled", label: t("badge.disabled"), param: "false" },
  ];

  return (
    <section>
      <PageHeader
        title={t("nav.endpoints")}
        subtitle={t("endpoints.subtitle")}
        actions={
          <button type="button" className="button" onClick={reload}>
            <Icon name="refresh" size={15} />
            {t("common.refresh")}
          </button>
        }
      />
      <div className="panel">
        <h2>{t("endpoints.add")}</h2>
        <CreateEndpoint
          create={(url, forCustomer) => api.createEndpoint(projectId, url, forCustomer)}
          withCustomer
          onCreated={(endpoint) => {
            setCreated(endpoint);
            update({ enabled: undefined });
            reload();
          }}
        />
      </div>
      {created && <SecretNotice url={created.url} secret={created.secret} onDismiss={() => setCreated(null)} />}
      <PortalLinkPanel projectId={projectId} />
      <div className="toolbar">
        <div className="segmented" role="group" aria-label={t("endpoints.filter")}>
          {filters.map((f) => (
            <button key={f.value} type="button" aria-pressed={filter === f.value} onClick={() => update({ enabled: f.param })}>
              {f.label}
            </button>
          ))}
        </div>
        <form
          className="search"
          role="search"
          onSubmit={(e) => {
            e.preventDefault();
            update({ customer: customerInput.trim() });
          }}
        >
          <Icon name="search" size={16} />
          <label htmlFor="endpoint-customer-filter" className="sr-only">
            {t("endpoints.customerFilter")}
          </label>
          <input id="endpoint-customer-filter" placeholder={t("endpoints.customerFilter")} value={customerInput} onChange={(e) => setCustomerInput(e.target.value)} />
        </form>
      </div>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" &&
        (state.data.endpoints.length === 0 && offset === 0 ? (
          <Empty title={filter === "all" ? t("endpoints.none") : t("endpoints.noneFiltered")}>{filter === "all" && t("endpoints.noneHint")}</Empty>
        ) : (
          <div className="panel panel-flush">
            <ul className="rows">
              {state.data.endpoints.map((endpoint) => (
                <EndpointRow key={endpoint.id} projectId={projectId} endpoint={endpoint} onChanged={reload} />
              ))}
            </ul>
            <Pager
              offset={offset}
              limit={limit}
              count={state.data.endpoints.length}
              nextOffset={state.data.next_offset}
              onOffsetChange={(o) => update({ offset: String(o) })}
              onLimitChange={(l) => update({ limit: String(l) })}
            />
          </div>
        ))}
    </section>
  );
}

export function CreateEndpoint({
  create,
  withCustomer = false,
  onCreated,
}: {
  create: (url: string, customer?: string) => Promise<{ url: string; secret: string }>;
  withCustomer?: boolean;
  onCreated: (e: { url: string; secret: string }) => void;
}) {
  const [url, setUrl] = useState("");
  const [customer, setCustomer] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const endpoint = await create(url.trim(), customer.trim() || undefined);
      setUrl("");
      setCustomer("");
      onCreated({ url: endpoint.url, secret: endpoint.secret });
    } catch (err) {
      setError(err instanceof Error ? endpointError(err) : t("endpoints.createFailed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="inline-form" onSubmit={submit}>
      <label htmlFor="endpoint-url" className="sr-only">
        {t("endpoints.urlLabel")}
      </label>
      <input id="endpoint-url" type="url" required placeholder="https://example.com/webhooks" value={url} onChange={(e) => setUrl(e.target.value)} />
      {withCustomer && (
        <>
          <label htmlFor="endpoint-customer" className="sr-only">
            {t("endpoints.customerLabel")}
          </label>
          <input id="endpoint-customer" className="input-narrow" maxLength={128} placeholder={t("endpoints.customerPlaceholder")} value={customer} onChange={(e) => setCustomer(e.target.value)} />
        </>
      )}
      <button type="submit" className="button button-primary" disabled={busy || url.trim() === ""}>
        {busy ? t("common.adding") : t("endpoints.add")}
      </button>
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
    </form>
  );
}

export function SecretNotice({ url, secret, onDismiss }: { url: string; secret: string; onDismiss: () => void }) {
  return (
    <div className="panel secret" role="status">
      <p>
        {t("endpoints.secretFor")} <code>{url}</code>. {t("endpoints.secretHint")}
      </p>
      <code className="secret-value">{secret}</code>
      <div className="actions">
        <CopyButton value={secret} />
        <button type="button" className="button" onClick={onDismiss}>
          {t("common.done")}
        </button>
      </div>
    </div>
  );
}

export type EndpointOps = {
  update: (id: string, change: { url?: string; enabled?: boolean }) => Promise<unknown>;
  secret: (id: string) => Promise<{ secret: string }>;
};

export function EndpointRow({ projectId, endpoint, onChanged, ops }: { projectId?: string; endpoint: Endpoint; onChanged: () => void; ops?: EndpointOps }) {
  const api = useApi();
  const operations: EndpointOps = ops ?? { update: (id, change) => api.updateEndpoint(id, change), secret: (id) => api.getEndpointSecret(id) };
  const [editing, setEditing] = useState(false);
  const [url, setUrl] = useState(endpoint.url);
  const [secret, setSecret] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function run(action: () => Promise<unknown>, after?: () => void) {
    setBusy(true);
    setError(null);
    try {
      await action();
      after?.();
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? endpointError(err) : t("common.requestFailed"));
    } finally {
      setBusy(false);
    }
  }

  async function revealSecret() {
    setError(null);
    try {
      setSecret((await operations.secret(endpoint.id)).secret);
    } catch (err) {
      setError(err instanceof Error ? describeError(err) : t("endpoints.secretFailed"));
    }
  }

  return (
    <li className="row">
      <div className="row-main">
        {editing ? (
          <form
            className="inline-form"
            onSubmit={(e) => {
              e.preventDefault();
              void run(() => operations.update(endpoint.id, { url: url.trim() }), () => setEditing(false));
            }}
          >
            <label htmlFor={`url-${endpoint.id}`} className="sr-only">
              {t("endpoints.urlLabel")}
            </label>
            <input id={`url-${endpoint.id}`} type="url" required value={url} onChange={(e) => setUrl(e.target.value)} />
            <button type="submit" className="button button-primary" disabled={busy}>
              {t("common.save")}
            </button>
            <button type="button" className="button" onClick={() => { setEditing(false); setUrl(endpoint.url); }}>
              {t("common.cancel")}
            </button>
          </form>
        ) : (
          <code className="url">{endpoint.url}</code>
        )}
        <div className="row-meta">
          <EnabledBadge enabled={endpoint.enabled} />
          <span className="muted">{t("endpoints.added", { date: formatTime(endpoint.created_at) })}</span>
          {projectId && <a href={href("projects", projectId, "deliveries") + `?endpoint_id=${endpoint.id}`}>{t("nav.deliveries")}</a>}
          {endpoint.customer && <span className="tag">{endpoint.customer}</span>}
        </div>
        {secret && (
          <div className="secret-inline">
            <code className="secret-value">{secret}</code>
            <CopyButton value={secret} />
            <button type="button" className="button" onClick={() => setSecret(null)}>
              {t("common.hide")}
            </button>
          </div>
        )}
        {error && (
          <p className="field-error" role="alert">
            {error}
          </p>
        )}
      </div>
      {!editing && (
        <div className="actions">
          <button type="button" className="button" disabled={busy} onClick={() => void run(() => operations.update(endpoint.id, { enabled: !endpoint.enabled }))}>
            {endpoint.enabled ? t("endpoints.disable") : t("endpoints.enable")}
          </button>
          <button type="button" className="button" onClick={() => setEditing(true)}>
            {t("endpoints.editUrl")}
          </button>
          {!secret && (
            <button type="button" className="button" onClick={() => void revealSecret()}>
              {t("endpoints.showSecret")}
            </button>
          )}
        </div>
      )}
    </li>
  );
}

function endpointError(error: Error): string {
  if ("code" in error) {
    switch ((error as { code: string }).code) {
      case "destination_not_allowed":
        return t("endpoint.destinationNotAllowed");
      case "invalid_input":
        return t("endpoint.invalidUrl");
    }
  }
  return describeError(error);
}
