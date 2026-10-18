import { useEffect, useState, type FormEvent } from "react";
import { ApiClient, ApiError, publicApi } from "../../shared/api/client";
import { describeError } from "../../shared/errors";
import { looksLikeAPIKey } from "../../shared/session";
import { productName } from "../../shared/product";
import { languages, t, useLang } from "../../shared/i18n";
import { CopyButton } from "../../shared/ui/CopyButton";

const features = [1, 2, 3, 4];

type ApiState = "checking" | "ready" | "down";

export function SignInPage({ onSignedIn, notice }: { onSignedIn: (key: string) => void; notice?: string }) {
  const { lang, setLang } = useLang();
  const [key, setKey] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);
  const [api, setApi] = useState<ApiState>("checking");
  const [development, setDevelopment] = useState(false);
  const [mode, setMode] = useState<"account" | "key">("account");

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
      setError(t("signin.malformed"));
      return;
    }
    setChecking(true);
    setError(null);
    try {
      // Any authenticated call proves the key works; projects are needed next anyway.
      await new ApiClient(trimmed).listProjects({ limit: 1 });
      onSignedIn(trimmed);
    } catch (err) {
      setError(err instanceof Error ? describeSignInError(err) : t("signin.failed"));
    } finally {
      setChecking(false);
    }
  }

  return (
    <main className="signin">
      <div className="segmented signin-lang" role="group" aria-label={t("nav.language")}>
        {languages.map((l) => (
          <button key={l.code} type="button" aria-pressed={lang === l.code} onClick={() => setLang(l.code)}>
            {l.label}
          </button>
        ))}
      </div>
      <div className="signin-layout">
        <section className="signin-intro" aria-labelledby="product-name">
          <p className="eyebrow">{t("signin.tagline")}</p>
          <h1 id="product-name">{productName}</h1>
          <p className="lead">{t("signin.lead", { name: productName })}</p>
          <ul className="features">
            {features.map((n) => (
              <li key={n}>
                <strong>{t(`signin.feature${n}.title` as never)}</strong>
                <span className="hint">{t(`signin.feature${n}.text` as never)}</span>
              </li>
            ))}
          </ul>
          <div className="signin-meta">
            <ApiStatus state={api} />
            <a href="/openapi.yaml" target="_blank" rel="noreferrer">
              {t("signin.apiReference")}
            </a>
          </div>
        </section>

        <div className="signin-side">
          <div className="panel signin-panel">
            <div className="segmented" role="group" aria-label={t("signin.title")}>
              <button type="button" aria-pressed={mode === "account"} onClick={() => setMode("account")}>
                {t("signin.tabAccount")}
              </button>
              <button type="button" aria-pressed={mode === "key"} onClick={() => setMode("key")}>
                {t("signin.tabKey")}
              </button>
            </div>
            {notice && (
              <p className="notice" role="status">
                {notice}
              </p>
            )}
            {mode === "account" ? (
              <AccountForm onSignedIn={onSignedIn} />
            ) : (
              <form onSubmit={submit}>
                <label htmlFor="api-key">{t("signin.keyLabel")}</label>
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
                  {checking ? t("common.checking") : t("signin.submit")}
                </button>
                <p className="hint">{t("signin.keyNote", { name: productName })}</p>
              </form>
            )}
          </div>

          {development ? (
            <DevelopmentKey onUse={onSignedIn} />
          ) : (
            <div className="panel signin-help">
              <h2>{t("signin.noKey")}</h2>
              <p className="hint">
                {t("signin.noKeyText")} <code>hooklane-admin bootstrap</code>
              </p>
            </div>
          )}
        </div>
      </div>
    </main>
  );
}

function AccountForm({ onSignedIn }: { onSignedIn: (token: string) => void }) {
  const [creating, setCreating] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [organization, setOrganization] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const session = creating ? await publicApi.signUp(email.trim(), password, organization.trim()) : await publicApi.signIn(email.trim(), password);
      onSignedIn(session.token);
    } catch (err) {
      setError(describeAccountError(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit}>
      <label htmlFor="account-email">{t("signin.email")}</label>
      <input id="account-email" type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} />
      <label htmlFor="account-password">{t("signin.password")}</label>
      <input
        id="account-password"
        type="password"
        autoComplete={creating ? "new-password" : "current-password"}
        value={password}
        onChange={(e) => setPassword(e.target.value)}
      />
      {creating && (
        <>
          <p className="hint">{t("signin.passwordHint")}</p>
          <label htmlFor="account-organization">{t("signin.organization")}</label>
          <input id="account-organization" type="text" autoComplete="organization" value={organization} onChange={(e) => setOrganization(e.target.value)} />
        </>
      )}
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
      <button type="submit" className="button button-primary" disabled={busy || email.trim() === "" || password === ""}>
        {busy ? t("common.checking") : creating ? t("signin.createAccount") : t("signin.submit")}
      </button>
      <button type="button" className="button button-ghost" onClick={() => setCreating(!creating)}>
        {creating ? t("signin.haveAccount") : t("signin.needAccount")}
      </button>
    </form>
  );
}

function describeAccountError(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case "invalid_credentials":
        return t("signin.invalidCredentials");
      case "email_taken":
        return t("signin.emailTaken");
      case "signup_disabled":
        return t("signin.signupDisabled");
      case "invalid_input":
        return t("signin.invalidInput");
      case "rate_limited":
        return t("signin.tooMany");
    }
    if (err.status === 429) return t("signin.tooMany");
    return describeError(err);
  }
  return t("signin.failed");
}

function ApiStatus({ state }: { state: ApiState }) {
  const label = state === "checking" ? t("signin.apiChecking") : state === "ready" ? t("signin.apiReady") : t("signin.apiDown");
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
      setError(err instanceof Error ? describeError(err) : t("signin.devFailed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="panel signin-help">
      <p className="eyebrow">{t("signin.devMode")}</p>
      <h2>{t("signin.noKey")}</h2>
      {rawKey ? (
        <>
          <p className="hint">{t("signin.devCopy")}</p>
          <code className="secret-value">{rawKey}</code>
          <div className="actions">
            <CopyButton value={rawKey} />
            <button type="button" className="button button-primary" onClick={() => onUse(rawKey)}>
              {t("signin.devContinue")}
            </button>
          </div>
        </>
      ) : (
        <>
          <p className="hint">{t("signin.devText")}</p>
          <button type="button" className="button" disabled={busy} onClick={() => void generate()}>
            {busy ? t("common.generating") : t("signin.devGenerate")}
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
    return t("signin.rejected");
  }
  return message;
}
