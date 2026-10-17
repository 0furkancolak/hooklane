import { useApi } from "../../shared/api/context";
import { href } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { formatRelative } from "../../shared/format";
import { ErrorState, Loading } from "../../shared/ui/States";

export function OverviewPage({ projectId }: { projectId: string }) {
  const api = useApi();
  const [state, reload] = useAsync((signal) => api.getDeliveryStats(projectId, signal), [api, projectId]);

  return (
    <section>
      <div className="section-head">
        <h1>Overview</h1>
        <button type="button" className="button" onClick={reload}>
          Refresh
        </button>
      </div>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" && (
        <>
          <div className="stats">
            <Stat label="Pending" value={state.data.pending} link={href("projects", projectId, "deliveries") + "?status=pending"} />
            <Stat label="Processing" value={state.data.processing} link={href("projects", projectId, "deliveries") + "?status=processing"} />
            <Stat label="Succeeded" value={state.data.succeeded} link={href("projects", projectId, "deliveries") + "?status=succeeded"} />
            <Stat
              label="Dead letter"
              value={state.data.dead_letter}
              tone={state.data.dead_letter > 0 ? "bad" : undefined}
              link={href("projects", projectId, "deliveries") + "?status=dead_letter"}
            />
          </div>
          <p className="hint">
            {state.data.oldest_due_at
              ? `The oldest due delivery has been waiting since ${formatRelative(state.data.oldest_due_at)}. If this keeps growing, check that a worker is running.`
              : "No deliveries are waiting for a worker."}
          </p>
        </>
      )}
    </section>
  );
}

function Stat({ label, value, link, tone }: { label: string; value: number; link: string; tone?: "bad" }) {
  return (
    <a className={`stat ${tone === "bad" ? "stat-bad" : ""}`} href={link}>
      <span className="stat-value">{value.toLocaleString()}</span>
      <span className="stat-label">{label}</span>
    </a>
  );
}
