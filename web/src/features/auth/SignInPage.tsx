import { useEffect, useState, type FormEvent } from "react";
import { ApiClient, publicApi } from "../../shared/api/client";
import { describeError } from "../../shared/errors";
import { looksLikeAPIKey } from "../../shared/session";
import { productName, productTagline } from "../../shared/product";
import { CopyButton } from "../../shared/ui/CopyButton";

const features: { title: string; text: string }[] = [
  { title: "Signed deliveries", text: "Every request carries a webhook-signature header your receivers can verify." },
  { title: "Retries and dead letters", text: "Failed deliveries back off and retry; the ones that never succeed are kept for review." },
  { title: "Full history and replay", text: "See each attempt's status, response and timing, and replay any finished delivery." },
  { title: "Projects and API keys", text: "Separate endpoints and events per project, each with its own keys." },
];

type ApiState = "checking" | "ready" | "down";

export function SignInPage({ onSignedIn, notice }: { onSignedIn: (key: string) => void; notice?: string }) {
  const [key, setKey] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);
  const [api, setApi] = useState<ApiState>("checking");
  const [development, setDevelopment] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    void publicApi.ready(controller.signal).then((ok) => {
      if (!controller.signal.aborted) setApi(ok ? "ready" : "down");
    });
    void publicApi.developmentMode(controller.signal).then((dev) => {
      if (!controller.signal.aborted) setDevelopment(dev);
    });
    return () => controller.abort();
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const trimmed = key.trim();
    if (!looksLikeAPIKey(trimmed)) {
      setError("That does not look like a Hooklane API key (hlk_ followed by 64 hex characters).");
      return;
    }
    setChecking(true);
    setError(null);
    try {
      // Any authenticated call proves the key works; projects are needed next anyway.
      await new ApiClient(trimmed).listProjects({ limit: 1 });
      onSignedIn(trimmed);
    } catch (err) {
      setError(err instanceof Error ? describeSignInError(err) : "Sign-in failed.");
    } finally {
      setChecking(false);
    }
  }

  return (
    <main className="signin">
      <div className="signin-layout">
        <section className="signin-intro" aria-labelledby="product-name">
          <p className="eyebrow">{productTagline}</p>
          <h1 id="product-name">{productName}</h1>
          <p className="lead">
            Send events to one API. {productName} delivers them to your endpoints with signed requests, retries and a complete delivery
            history.
          </p>
          <ul className="features">
            {features.map((feature) => (
              <li key={feature.title}>
                <strong>{feature.title}</strong>
                <span className="hint">{feature.text}</span>
              </li>
            ))}
          </ul>
          <div className="signin-meta">
            <ApiStatus state={api} />
            <a href="/openapi.yaml" target="_blank" rel="noreferrer">
              API reference (OpenAPI)
            </a>
          </div>
        </section>

        <div className="signin-side">
          <form className="panel signin-panel" onSubmit={submit}>
            <h2>Sign in</h2>
            {notice && (
              <p className="notice" role="status">
                {notice}
              </p>
            )}
            <label htmlFor="api-key">API key</label>
            <input
              id="api-key"
              type="password"
              autoComplete="off"
              spellCheck={false}
              placeholder="hlk_…"
              value={key}
              onChange={(e) => setKey(e.target.value)}
            />
            {error && (
              <p className="field-error" role="alert">
                {error}
              </p>
            )}
            <button type="submit" className="button button-primary" disabled={checking || key.trim() === ""}>
              {checking ? "Checking…" : "Sign in"}
            </button>
            <p className="hint">The key is kept in this tab's session storage only and is sent solely to this {productName} API.</p>
          </form>

          {development ? (
            <DevelopmentKey onUse={onSignedIn} />
          ) : (
            <div className="panel signin-help">
              <h2>No key yet?</h2>
              <p className="hint">
                Create the first one on the server with <code>hooklane-admin bootstrap</code>. Once signed in, generate more keys from a
                project's <strong>API keys</strong> page.
              </p>
            </div>
          )}
        </div>
      </div>
    </main>
  );
}

function ApiStatus({ state }: { state: ApiState }) {
  const label = state === "checking" ? "Checking the API…" : state === "ready" ? "API is ready" : "API is not reachable";
  return (
    <span className={`api-status api-status-${state}`} role="status">
      <span className="dot" aria-hidden="true" />
      {label}
    </span>
  );
}

function DevelopmentKey({ onUse }: { onUse: (key: string) => void }) {
  const [rawKey, setRawKey] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function generate() {
    setBusy(true);
    setError(null);
    try {
      setRawKey((await publicApi.issueDevelopmentKey()).raw_key);
    } catch (err) {
      setError(err instanceof Error ? describeError(err) : "Could not generate a key.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="panel signin-help">
      <p className="eyebrow">Development mode</p>
      <h2>No key yet?</h2>
      {rawKey ? (
        <>
          <p className="hint">Copy this key if you want to call the API yourself; it will not be shown again.</p>
          <code className="secret-value">{rawKey}</code>
          <div className="actions">
            <CopyButton value={rawKey} />
            <button type="button" className="button button-primary" onClick={() => onUse(rawKey)}>
              Continue to dashboard
            </button>
          </div>
        </>
      ) : (
        <>
          <p className="hint">
            This API runs with <code>APP_ENV=development</code>, so this machine can generate a key for the first project without the
            admin CLI.
          </p>
          <button type="button" className="button" disabled={busy} onClick={() => void generate()}>
            {busy ? "Generating…" : "Generate a development key"}
          </button>
        </>
      )}
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

function describeSignInError(error: Error): string {
  const message = describeError(error);
  if ("status" in error && (error as { status: number }).status === 401) {
    return "The API rejected this key. It may be revoked or mistyped.";
  }
  return message;
}
