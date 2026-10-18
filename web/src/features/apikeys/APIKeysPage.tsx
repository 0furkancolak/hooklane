import { useState, type FormEvent } from "react";
import { useApi } from "../../shared/api/context";
import type { APIKey } from "../../shared/api/types";
import { useAsync } from "../../shared/useAsync";
import { formatTime } from "../../shared/format";
import { describeError } from "../../shared/errors";
import { t } from "../../shared/i18n";
import { Icon } from "../../shared/ui/Icon";
import { PageHeader } from "../../shared/ui/PageHeader";
import { Empty, ErrorState, Loading } from "../../shared/ui/States";
import { CopyButton } from "../../shared/ui/CopyButton";

export function APIKeysPage({ projectId }: { projectId: string }) {
  const api = useApi();
  const [state, reload] = useAsync((signal) => api.listAPIKeys(projectId, signal), [api, projectId]);
  const [created, setCreated] = useState<{ name: string; rawKey: string } | null>(null);

  return (
    <section>
      <PageHeader
        title={t("nav.apiKeys")}
        subtitle={t("apikeys.subtitle")}
        actions={
          <button type="button" className="button" onClick={reload}>
            <Icon name="refresh" size={15} />
            {t("common.refresh")}
          </button>
        }
      />
      <div className="panel">
        <h2>{t("apikeys.generateTitle")}</h2>
        <CreateAPIKey
          projectId={projectId}
          onCreated={(key) => {
            setCreated(key);
            reload();
          }}
        />
      </div>
      {created && <RawKeyNotice name={created.name} rawKey={created.rawKey} onDismiss={() => setCreated(null)} />}
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" &&
        (state.data.api_keys.length === 0 ? (
          <Empty title={t("apikeys.none")}>{t("apikeys.noneHint")}</Empty>
        ) : (
          <ul className="rows panel panel-flush">
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
      setError(err instanceof Error ? describeError(err) : t("apikeys.createFailed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="inline-form" onSubmit={submit}>
      <label htmlFor="api-key-name" className="sr-only">
        {t("apikeys.nameLabel")}
      </label>
      <input id="api-key-name" required maxLength={200} placeholder={t("apikeys.namePlaceholder")} value={name} onChange={(e) => setName(e.target.value)} />
      <button type="submit" className="button button-primary" disabled={busy || name.trim() === ""}>
        {busy ? t("common.generating") : t("apikeys.generate")}
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
        {t("apikeys.secretNotice", { name })}
      </p>
      <code className="secret-value">{rawKey}</code>
      <div className="actions">
        <CopyButton value={rawKey} />
        <button type="button" className="button" onClick={onDismiss}>
          {t("common.done")}
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
    if (!window.confirm(t("apikeys.revokeConfirm", { name: apiKey.name }))) {
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api.revokeAPIKey(apiKey.id);
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? describeError(err) : t("apikeys.revokeFailed"));
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
          <span className={`badge ${apiKey.revoked ? "badge-muted" : "badge-succeeded"}`}>{apiKey.revoked ? t("badge.revoked") : t("badge.active")}</span>
          <span className="muted">{t("projects.created", { date: formatTime(apiKey.created_at) })}</span>
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
            {t("apikeys.revoke")}
          </button>
        </div>
      )}
    </li>
  );
}
