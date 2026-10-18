import { Icon, type IconName } from "./Icon";

const steps: { icon: IconName; title: string; text: string }[] = [
  { icon: "events", title: "1. Your app sends one event", text: "A single POST to Hooklane, for example invoice.paid, with any JSON payload." },
  { icon: "bolt", title: "2. Hooklane delivers it", text: "It signs the request and calls every endpoint you registered, in parallel." },
  { icon: "repeat", title: "3. Failures are handled", text: "A slow or broken receiver gets retried with backoff. Nothing is lost." },
  { icon: "search", title: "4. You see everything", text: "Every attempt, response and error is recorded, and any delivery can be replayed." },
];

const reasons = [
  "You stop writing retry queues, signing code and delivery logs for every integration.",
  "Receivers can verify each request came from you (Standard Webhooks signatures).",
  "Private network addresses are blocked, so customer-supplied URLs cannot reach your internals.",
  "It is self-hosted: events and secrets stay in your own PostgreSQL.",
];

/** Says what Hooklane is for and why to use it, in plain words. */
export function Explainer() {
  return (
    <section className="panel explainer" aria-labelledby="explainer-title">
      <h2 id="explainer-title">What is Hooklane for?</h2>
      <p className="lead">
        When something happens in your product (a payment, a signup, a shipped order) other systems often need to know. Hooklane is the service that
        reliably tells them, by sending webhooks on your behalf.
      </p>
      <ol className="steps">
        {steps.map((step) => (
          <li key={step.title}>
            <span className="step-icon">
              <Icon name={step.icon} />
            </span>
            <strong>{step.title}</strong>
            <span className="muted">{step.text}</span>
          </li>
        ))}
      </ol>
      <h3>Why use it</h3>
      <ul className="reasons">
        {reasons.map((reason) => (
          <li key={reason}>
            <Icon name="check" size={16} />
            {reason}
          </li>
        ))}
      </ul>
    </section>
  );
}
