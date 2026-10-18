import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { publicApi } from "../../shared/api/client";
import { useApi } from "../../shared/api/context";
import { isTerminal, type Attempt, type Delivery, type IngestedEvent, type SinkRequest } from "../../shared/api/types";
import { describeError } from "../../shared/errors";
import { formatTime, shortId } from "../../shared/format";
import { href } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { Icon } from "../../shared/ui/Icon";
import { HttpStatus, StatusBadge } from "../../shared/ui/StatusBadge";
import { eventTemplates, formatPayload } from "./templates";
import { useReceiver } from "./useReceiver";

type ReceiverMode = "builtin" | "custom";
type Behaviour = "ok" | "fail";

const pollIntervalMs = 1500;
const pollLimitMs = 3 * 60 * 1000;

function endpointStorageKey(projectId: string) {
  return `hooklane.playground.endpoint.${projectId}`;
}

function rememberedEndpoint(projectId: string): string | null {
  try {
    return localStorage.getItem(endpointStorageKey(projectId));
  } catch {
    return null;
  }
}

function rememberEndpoint(projectId: string, id: string) {
  try {
    localStorage.setItem(endpointStorageKey(projectId), id);
  } catch {
    // The playground still works without remembering its endpoint.
  }
}

export function PlaygroundPage({ projectId }: { projectId: string }) {
  const api = useApi();
  const receiver = useReceiver();
  const [choice, setMode] = useState<ReceiverMode | null>(null);
  const [behaviour, setBehaviour] = useState<Behaviour>("ok");
  const [customUrl, setCustomUrl] = useState("");
  const [templateIndex, setTemplateIndex] = useState(0);
  const [eventType, setEventType] = useState(eventTemplates[0].eventType);
  const [payload, setPayload] = useState(formatPayload(eventTemplates[0].payload));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState<{ event: IngestedEvent; url: string; secret?: string } | null>(null);

  const [endpoints] = useAsync((signal) => api.listEndpoints(projectId, { limit: 100 }, signal), [api, projectId]);
  const ownId = rememberedEndpoint(projectId);
  const otherEndpoints = endpoints.status === "ready" ? endpoints.data.endpoints.filter((e) => e.enabled && e.id !== ownId).length : 0;

  const builtinAvailable = receiver.sinkUrl !== null;
  const mode: ReceiverMode = choice ?? (builtinAvailable ? "builtin" : "custom");

  const receiverUrl = useMemo(() => {
    if (mode === "custom") return customUrl.trim();
    return `${receiver.sinkUrl}/${receiver.token}${behaviour === "fail" ? "?status=503" : ""}`;
  }, [mode, customUrl, receiver.sinkUrl, receiver.token, behaviour]);

  function chooseTemplate(index: number) {
    setTemplateIndex(index);
    setEventType(eventTemplates[index].eventType);
    setPayload(formatPayload(eventTemplates[index].payload));
  }

  /** Reuses the playground's endpoint, creating it or pointing it at the chosen URL when needed. */
  async function prepareEndpoint(): Promise<{ secret?: string }> {
    const id = rememberedEndpoint(projectId);
    if (id) {
      try {
        await api.updateEndpoint(id, { url: receiverUrl, enabled: true });
        return {};
      } catch {
        // The endpoint was deleted or belongs elsewhere; fall through and create a new one.
      }
    }
    const created = await api.createEndpoint(projectId, receiverUrl);
    rememberEndpoint(projectId, created.id);
    return { secret: created.secret };
  }

  async function send(event: FormEvent) {
    event.preventDefault();
    setError(null);
    let parsed: unknown;
    try {
      parsed = JSON.parse(payload);
    } catch {
      setError("The payload is not valid JSON.");
      return;
    }
    setBusy(true);
    try {
      const { secret } = await prepareEndpoint();
      const ingested = await api.ingestEvent(projectId, { event_type: eventType.trim(), payload: parsed });
      setSent({ event: ingested, url: receiverUrl, secret });
    } catch (err) {
      setError(err instanceof Error ? describeError(err) : "Could not send the event.");
    } finally {
      setBusy(false);
    }
  }

  const canSend = !busy && eventType.trim() !== "" && (mode === "builtin" || customUrl.trim() !== "");

  return (
    <section className="playground">
      <div className="section-head">
        <div>
          <h1>Playground</h1>
          <p className="subtitle">Send a test event and watch Hooklane deliver it. Nothing here is hypothetical: these are real events, deliveries and attempts.</p>
        </div>
      </div>

      {otherEndpoints > 0 && (
        <p className="notice" role="note">
          This project already has {otherEndpoints} other enabled {otherEndpoints === 1 ? "endpoint" : "endpoints"}, and every event goes to all of them. For a clean demo, use a project of its own.
        </p>
      )}

      <form className="play-grid" onSubmit={send}>
        <div className="panel play-step">
          <h2>
            <span className="step-no">1</span> Choose a receiver
          </h2>
          <p className="muted">The receiver is the URL that should be told about events. In real use it is your customer's server.</p>
          <div className="choice" role="radiogroup" aria-label="Receiver">
            <label aria-label="Built-in receiver" className={`choice-item ${mode === "builtin" ? "is-selected" : ""} ${builtinAvailable ? "" : "is-disabled"}`}>
              <input type="radio" name="receiver" checked={mode === "builtin"} disabled={!builtinAvailable} onChange={() => setMode("builtin")} />
              <span>
                <strong>Built-in receiver</strong>
                <span className="muted">
                  {receiver.loading
                    ? "Checking…"
                    : builtinAvailable
                      ? "Catches the request so you can inspect it. Development mode only."
                      : "Available when the API runs with APP_ENV=development."}
                </span>
              </span>
            </label>
            <label aria-label="My own URL" className={`choice-item ${mode === "custom" ? "is-selected" : ""}`}>
              <input type="radio" name="receiver" checked={mode === "custom"} onChange={() => setMode("custom")} />
              <span>
                <strong>My own URL</strong>
                <span className="muted">Any public https URL, for example one from webhook.site.</span>
              </span>
            </label>
          </div>
          {mode === "custom" ? (
            <div className="field">
              <label htmlFor="play-url">Receiver URL</label>
              <input id="play-url" type="url" placeholder="https://webhook.site/your-id" value={customUrl} onChange={(e) => setCustomUrl(e.target.value)} />
            </div>
          ) : (
            <div className="field">
              <label htmlFor="play-behaviour">Receiver behaviour</label>
              <select id="play-behaviour" value={behaviour} onChange={(e) => setBehaviour(e.target.value as Behaviour)}>
                <option value="ok">Answer 200 (success)</option>
                <option value="fail">Answer 503 (watch the retries)</option>
              </select>
            </div>
          )}
        </div>

        <div className="panel play-step">
          <h2>
            <span className="step-no">2</span> Write an event
          </h2>
          <p className="muted">An event is a type plus a JSON payload. Hooklane delivers it to every enabled endpoint of this project.</p>
          <div className="field">
            <label htmlFor="play-template">Start from</label>
            <select id="play-template" value={templateIndex} onChange={(e) => chooseTemplate(Number(e.target.value))}>
              {eventTemplates.map((t, i) => (
                <option key={t.eventType} value={i}>
                  {t.label}
                </option>
              ))}
            </select>
          </div>
          <div className="field">
            <label htmlFor="play-type">Event type</label>
            <input id="play-type" required maxLength={200} value={eventType} onChange={(e) => setEventType(e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="play-payload">Payload (JSON)</label>
            <textarea id="play-payload" rows={7} spellCheck={false} value={payload} onChange={(e) => setPayload(e.target.value)} />
          </div>
        </div>

        <div className="panel play-step play-send">
          <h2>
            <span className="step-no">3</span> Send it
          </h2>
          <p className="muted">
            One request to <code>POST /v1/projects/…/events</code>. Hooklane answers immediately and delivers in the background.
          </p>
          <button type="submit" className="button button-primary" disabled={!canSend}>
            <Icon name="play" size={16} />
            {busy ? "Sending…" : "Send test event"}
          </button>
          {error && (
            <p className="field-error" role="alert">
              {error}
            </p>
          )}
        </div>
      </form>

      {sent && <Result key={sent.event.id} projectId={projectId} sent={sent} token={receiver.token} showReceived={mode === "builtin" && builtinAvailable} />}
    </section>
  );
}

type Row = { delivery: Delivery; attempts: Attempt[] };

function Result({ projectId, sent, token, showReceived }: { projectId: string; sent: { event: IngestedEvent; url: string; secret?: string }; token: string; showReceived: boolean }) {
  const api = useApi();
  const [rows, setRows] = useState<Row[]>([]);
  const [received, setReceived] = useState<SinkRequest[]>([]);
  const [finished, setFinished] = useState(false);
  const [startedAt] = useState(() => Date.now());

  const refresh = useCallback(
    async (signal: AbortSignal) => {
      const { deliveries } = await api.listEventDeliveries(sent.event.id, signal);
      const next = await Promise.all(
        deliveries.map(async (delivery) => ({ delivery, attempts: delivery.attempt_count > 0 ? (await api.listAttempts(delivery.id, signal)).attempts : [] })),
      );
      setRows(next);
      if (showReceived) setReceived(await publicApi.sinkRequests(token, signal));
      const done = next.length > 0 && next.every((r) => isTerminal(r.delivery.status));
      if (done || Date.now() - startedAt > pollLimitMs) setFinished(true);
    },
    [api, sent.event.id, showReceived, token, startedAt],
  );

  useEffect(() => {
    if (finished) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const tick = () => {
      refresh(controller.signal)
        .catch(() => undefined)
        .finally(() => {
          if (!controller.signal.aborted) timer = setTimeout(tick, pollIntervalMs);
        });
    };
    tick();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [refresh, finished]);

  const waiting = rows.some((r) => !isTerminal(r.delivery.status));

  return (
    <div className="panel result" aria-live="polite">
      <h2>What happened</h2>
      <ol className="timeline">
        <li>
          <strong>Event accepted.</strong> <code>{sent.event.event_type}</code> was stored as{" "}
          <a href={href("projects", projectId, "events", sent.event.id)}>
            <code>{shortId(sent.event.id)}</code>
          </a>{" "}
          and queued for <strong>{sent.event.delivery_count}</strong> {sent.event.delivery_count === 1 ? "endpoint" : "endpoints"}.
        </li>
        {rows.map(({ delivery, attempts }) => (
          <li key={delivery.id}>
            <strong>Delivery</strong>{" "}
            <a href={href("projects", projectId, "deliveries", delivery.id)}>
              <code>{shortId(delivery.id)}</code>
            </a>{" "}
            <StatusBadge status={delivery.status} />
            {attempts.length === 0 && !isTerminal(delivery.status) && <span className="muted"> waiting for the worker to pick it up…</span>}
            <ul className="attempts">
              {[...attempts]
                .sort((a, b) => a.attempt_number - b.attempt_number)
                .map((attempt) => (
                  <li key={attempt.id}>
                    Attempt {attempt.attempt_number}: <HttpStatus code={attempt.status_code} /> {attempt.error && <span className="muted">{attempt.error} </span>}
                    <span className="muted">
                      {attempt.duration_ms !== null ? `${attempt.duration_ms} ms, ` : ""}
                      {formatTime(attempt.attempted_at)}
                    </span>
                  </li>
                ))}
            </ul>
            {delivery.status === "pending" && delivery.attempt_count > 0 && (
              <p className="muted">Failed, so Hooklane retries with a growing delay between attempts. Open the delivery to follow the retries.</p>
            )}
            {delivery.status === "succeeded" && <p className="muted">The receiver answered with a 2xx, so the delivery is complete.</p>}
          </li>
        ))}
        {rows.length === 0 && sent.event.delivery_count === 0 && (
          <li className="muted">No deliveries were created, so this project has no enabled endpoint.</li>
        )}
      </ol>
      {waiting && !finished && <p className="muted">Checking every second or so…</p>}
      {sent.secret && (
        <p className="muted">
          A new endpoint was created for <code>{sent.url}</code>. Its signing secret is available on the <a href={href("projects", projectId, "endpoints")}>Endpoints</a> page.
        </p>
      )}
      {showReceived && <Received requests={received} />}
    </div>
  );
}

function Received({ requests }: { requests: SinkRequest[] }) {
  if (requests.length === 0) return <p className="muted">The built-in receiver has not been called yet.</p>;
  const latest = requests[0];
  const interesting = Object.entries(latest.headers).filter(([name]) => /^webhook-|^content-type$|^user-agent$/i.test(name));
  return (
    <div className="received">
      <h3>What the receiver got (latest request)</h3>
      <dl className="kv">
        {interesting.map(([name, value]) => (
          <div key={name}>
            <dt>{name}</dt>
            <dd>
              <code>{value}</code>
            </dd>
          </div>
        ))}
      </dl>
      <pre className="code-block">{prettyBody(latest.body)}</pre>
      <p className="muted">
        The <code>webhook-signature</code> header lets a real receiver prove the request came from Hooklane.
      </p>
    </div>
  );
}

function prettyBody(body: string): string {
  try {
    return JSON.stringify(JSON.parse(body), null, 2);
  } catch {
    return body;
  }
}
