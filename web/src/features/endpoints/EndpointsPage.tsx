import { useState, type FormEvent } from "react";
import { useApi } from "../../shared/api/context";
import type { Endpoint } from "../../shared/api/types";
import { href } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { formatTime } from "../../shared/format";
import { describeError } from "../../shared/errors";
import { Empty, ErrorState, Loading } from "../../shared/ui/States";
import { EnabledBadge } from "../../shared/ui/StatusBadge";
import { Pager } from "../../shared/ui/Pager";

const pageSize = 25;

export function EndpointsPage({ projectId }: { projectId: string }) {
  const api = useApi();
  const [offset, setOffset] = useState(0);
  const [state, reload] = useAsync((signal) => api.listEndpoints(projectId, { limit: pageSize, offset }, signal), [api, projectId, offset]);
  const [created, setCreated] = useState<{ url: string; secret: string } | null>(null);

  return (
    <section>
      <h1>Endpoints</h1>
      <CreateEndpoint
        projectId={projectId}
        onCreated={(endpoint) => {
          setCreated(endpoint);
          setOffset(0);
          reload();
        }}
      />
      {created && <SecretNotice url={created.url} secret={created.secret} onDismiss={() => setCreated(null)} />}
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" &&
        (state.data.endpoints.length === 0 && offset === 0 ? (
          <Empty title="No endpoints yet">Add the URL that should receive this project's webhooks.</Empty>
        ) : (
          <>
            <ul className="rows">
              {state.data.endpoints.map((endpoint) => (
                <EndpointRow key={endpoint.id} projectId={projectId} endpoint={endpoint} onChanged={reload} />
              ))}
            </ul>
            <Pager offset={offset} limit={pageSize} nextOffset={state.data.next_offset} onChange={setOffset} />
          </>
        ))}
    </section>
  );
}

function CreateEndpoint({ projectId, onCreated }: { projectId: string; onCreated: (e: { url: string; secret: string }) => void }) {
  const api = useApi();
  const [url, setUrl] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const endpoint = await api.createEndpoint(projectId, url.trim());
      setUrl("");
      onCreated({ url: endpoint.url, secret: endpoint.secret });
    } catch (err) {
      setError(err instanceof Error ? endpointError(err) : "Could not create the endpoint.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="inline-form" onSubmit={submit}>
      <label htmlFor="endpoint-url" className="sr-only">
        Endpoint URL
      </label>
      <input id="endpoint-url" type="url" required placeholder="https://example.com/webhooks" value={url} onChange={(e) => setUrl(e.target.value)} />
      <button type="submit" className="button button-primary" disabled={busy || url.trim() === ""}>
        {busy ? "Adding…" : "Add endpoint"}
      </button>
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
    </form>
  );
}

function SecretNotice({ url, secret, onDismiss }: { url: string; secret: string; onDismiss: () => void }) {
  return (
    <div className="panel secret" role="status">
      <p>
        Signing secret for <code>{url}</code>. Configure your receiver with it to verify the <code>webhook-signature</code> header.
      </p>
      <code className="secret-value">{secret}</code>
      <div className="actions">
        <CopyButton value={secret} />
        <button type="button" className="button" onClick={onDismiss}>
          Done
        </button>
      </div>
    </div>
  );
}

function EndpointRow({ projectId, endpoint, onChanged }: { projectId: string; endpoint: Endpoint; onChanged: () => void }) {
  const api = useApi();
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
      setError(err instanceof Error ? endpointError(err) : "Request failed.");
    } finally {
      setBusy(false);
    }
  }

  async function revealSecret() {
    setError(null);
    try {
      setSecret((await api.getEndpointSecret(endpoint.id)).secret);
    } catch (err) {
      setError(err instanceof Error ? describeError(err) : "Could not load the secret.");
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
              void run(() => api.updateEndpoint(endpoint.id, { url: url.trim() }), () => setEditing(false));
            }}
          >
            <label htmlFor={`url-${endpoint.id}`} className="sr-only">
              Endpoint URL
            </label>
            <input id={`url-${endpoint.id}`} type="url" required value={url} onChange={(e) => setUrl(e.target.value)} />
            <button type="submit" className="button button-primary" disabled={busy}>
              Save
            </button>
            <button type="button" className="button" onClick={() => { setEditing(false); setUrl(endpoint.url); }}>
              Cancel
            </button>
          </form>
        ) : (
          <code className="url">{endpoint.url}</code>
        )}
        <div className="row-meta">
          <EnabledBadge enabled={endpoint.enabled} />
          <span className="muted">Added {formatTime(endpoint.created_at)}</span>
          <a href={href("projects", projectId, "deliveries") + `?endpoint_id=${endpoint.id}`}>Deliveries</a>
        </div>
        {secret && (
          <div className="secret-inline">
            <code className="secret-value">{secret}</code>
            <CopyButton value={secret} />
            <button type="button" className="button" onClick={() => setSecret(null)}>
              Hide
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
          <button type="button" className="button" disabled={busy} onClick={() => void run(() => api.updateEndpoint(endpoint.id, { enabled: !endpoint.enabled }))}>
            {endpoint.enabled ? "Disable" : "Enable"}
          </button>
          <button type="button" className="button" onClick={() => setEditing(true)}>
            Edit URL
          </button>
          {!secret && (
            <button type="button" className="button" onClick={() => void revealSecret()}>
              Show secret
            </button>
          )}
        </div>
      )}
    </li>
  );
}

function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      className="button"
      onClick={() => {
        void navigator.clipboard?.writeText(value).then(() => {
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        });
      }}
    >
      {copied ? "Copied" : "Copy"}
    </button>
  );
}

function endpointError(error: Error): string {
  if ("code" in error) {
    switch ((error as { code: string }).code) {
      case "destination_not_allowed":
        return "That URL points to a private, internal or reserved address, which this deployment does not allow.";
      case "invalid_input":
        return "Enter a valid https:// URL (http:// is only accepted in development).";
    }
  }
  return describeError(error);
}
