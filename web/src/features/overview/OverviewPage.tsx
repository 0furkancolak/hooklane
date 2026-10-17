import { productName, productTagline } from "../../shared/product";
import { useEffect, useState } from "react";

type HealthState = "checking" | "ok" | "down";

export function OverviewPage() {
  const [health, setHealth] = useState<HealthState>("checking");

  useEffect(() => {
    let cancelled = false;

    fetch("/healthz")
      .then((response) => {
        if (!cancelled) {
          setHealth(response.ok ? "ok" : "down");
        }
      })
      .catch(() => {
        if (!cancelled) {
          setHealth("down");
        }
      });

    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <main className="page">
      <header>
        <p className="eyebrow">{productTagline}</p>
        <h1>{productName}</h1>
      </header>
      <section className="panel">
        <h2>API health</h2>
        <p>
          Status: <strong data-testid="health-status">{healthLabel(health)}</strong>
        </p>
        <p className="hint">
          Management APIs live under <code>/v1</code>. Bootstrap the first
          administrator with the admin CLI, then send requests with a Bearer API
          key.
        </p>
      </section>
    </main>
  );
}

function healthLabel(health: HealthState): string {
  switch (health) {
    case "checking":
      return "Checking";
    case "ok":
      return "OK";
    case "down":
      return "Unavailable";
    default: {
      const exhaustive: never = health;
      return exhaustive;
    }
  }
}
