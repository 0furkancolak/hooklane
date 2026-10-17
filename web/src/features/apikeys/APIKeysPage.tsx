import { useState, type FormEvent } from "react";
import { useApi } from "../../shared/api/context";
import type { APIKey } from "../../shared/api/types";
import { useAsync } from "../../shared/useAsync";
import { formatTime } from "../../shared/format";
import { describeError } from "../../shared/errors";
import { Empty, ErrorState, Loading } from "../../shared/ui/States";
import { CopyButton } from "../../shared/ui/CopyButton";

export function APIKeysPage({ projectId }: { projectId: string }) {
  const api = useApi();
  const [state, reload] = useAsync((signal) => api.listAPIKeys(projectId, signal), [api, projectId]);
  const [created, setCreated] = useState<{ name: string; rawKey: string } | null>(null);

  return (
    <section>
      <h1>API keys</h1>
      <p className="hint">Any key of this organization can manage all of its projects. The full key is shown only once, right after it is generated.</p>
      <CreateAPIKey
        projectId={projectId}
        onCreated={(key) => {
          setCreated(key);
          reload();
        }}
      />
      {created && <RawKeyNotice name={created.name} rawKey={created.rawKey} onDismiss={() => setCreated(null)} />}
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" &&
        (state.data.api_keys.length === 0 ? (
          <Empty title="No API keys yet">Generate a key for each service or person that calls the Hooklane API.</Empty>
        ) : (
          <ul className="rows">
            {state.data.api_keys.map((key) => (
              <APIKeyRow key={key.id} apiKey={key} onChanged={reload} />
            ))}
          </ul>
        ))}
    </section>
  );
}

function CreateAPIKey({ projectId, onCreated }: { projectId: string; onCreated: (key: { name: string; rawKey: string }) => void }) {
  const api = useApi();
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const key = await api.createAPIKey(projectId, name.trim());
      setName("");
      onCreated({ name: key.name, rawKey: key.raw_key });
    } catch (err) {
      setError(err instanceof Error ? describeError(err) : "Could not generate the API key.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="inline-form" onSubmit={submit}>
      <label htmlFor="api-key-name" className="sr-only">
        Key name
      </label>
      <input id="api-key-name" required maxLength={200} placeholder="Key name, e.g. billing-service" value={name} onChange={(e) => setName(e.target.value)} />
      <button type="submit" className="button button-primary" disabled={busy || name.trim() === ""}>
        {busy ? "Generating…" : "Generate API key"}
      </button>
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
    </form>
  );
}

function RawKeyNotice({ name, rawKey, onDismiss }: { name: string; rawKey: string; onDismiss: () => void }) {
  return (
    <div className="panel secret" role="status">
      <p>
        API key <strong>{name}</strong>. Copy it now and store it somewhere safe: it will not be shown again.
      </p>
      <code className="secret-value">{rawKey}</code>
      <div className="actions">
        <CopyButton value={rawKey} />
        <button type="button" className="button" onClick={onDismiss}>
          Done
        </button>
      </div>
    </div>
  );
}

function APIKeyRow({ apiKey, onChanged }: { apiKey: APIKey; onChanged: () => void }) {
  const api = useApi();
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function revoke() {
    if (!window.confirm(`Revoke the API key "${apiKey.name}"? Requests using it will be rejected. If you signed in with it, you will be signed out.`)) {
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api.revokeAPIKey(apiKey.id);
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? describeError(err) : "Could not revoke the API key.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <li className="row">
      <div className="row-main">
        <strong>{apiKey.name}</strong>
        <div className="row-meta">
          <code>{apiKey.prefix}…</code>
          <span className={`badge ${apiKey.revoked ? "badge-muted" : "badge-succeeded"}`}>{apiKey.revoked ? "revoked" : "active"}</span>
          <span className="muted">Created {formatTime(apiKey.created_at)}</span>
        </div>
        {error && (
          <p className="field-error" role="alert">
            {error}
          </p>
        )}
      </div>
      {!apiKey.revoked && (
        <div className="actions">
          <button type="button" className="button" disabled={busy} onClick={() => void revoke()}>
            Revoke
          </button>
        </div>
      )}
    </li>
  );
}
