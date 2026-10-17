import { useState, type FormEvent } from "react";
import { ApiClient } from "../../shared/api/client";
import { describeError } from "../../shared/errors";
import { looksLikeAPIKey } from "../../shared/session";
import { productName, productTagline } from "../../shared/product";

export function SignInPage({ onSignedIn, notice }: { onSignedIn: (key: string) => void; notice?: string }) {
  const [key, setKey] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);

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
      <form className="panel signin-panel" onSubmit={submit}>
        <p className="eyebrow">{productTagline}</p>
        <h1>{productName}</h1>
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
        <p className="hint">
          The key is kept in this tab's session storage only and is sent solely to this Hooklane API. Create one with{" "}
          <code>admin bootstrap</code> or <code>POST /v1/projects/&#123;id&#125;/api-keys</code>.
        </p>
      </form>
    </main>
  );
}

function describeSignInError(error: Error): string {
  const message = describeError(error);
  if ("status" in error && (error as { status: number }).status === 401) {
    return "The API rejected this key. It may be revoked or mistyped.";
  }
  return message;
}
