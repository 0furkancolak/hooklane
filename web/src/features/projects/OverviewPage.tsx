import type { DeliveryStats, DeliveryStatus } from "../../shared/api/types";
import { useApi } from "../../shared/api/context";
import { href } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { formatRelative } from "../../shared/format";
import { Icon, type IconName } from "../../shared/ui/Icon";
import { ErrorState, Loading } from "../../shared/ui/States";
import { DeliveryTable } from "../deliveries/DeliveryTable";

const tiles: { status: DeliveryStatus; label: string; icon: IconName }[] = [
  { status: "pending", label: "Pending", icon: "clock" },
  { status: "processing", label: "Processing", icon: "loader" },
  { status: "succeeded", label: "Succeeded", icon: "check" },
  { status: "dead_letter", label: "Dead letter", icon: "alert" },
];

export function OverviewPage({ projectId }: { projectId: string }) {
  const api = useApi();
  const [state, reload] = useAsync((signal) => api.getDeliveryStats(projectId, signal), [api, projectId]);

  return (
    <section>
      <div className="section-head">
        <div>
          <h1>Overview</h1>
          <p className="subtitle">Delivery health for this project, straight from the queue.</p>
        </div>
        <button type="button" className="button" onClick={reload}>
          <Icon name="refresh" size={15} />
          Refresh
        </button>
      </div>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" && <Stats projectId={projectId} stats={state.data} />}
    </section>
  );
}

function Stats({ projectId, stats }: { projectId: string; stats: DeliveryStats }) {
  const total = stats.pending + stats.processing + stats.succeeded + stats.dead_letter;
  const finished = stats.succeeded + stats.dead_letter;
  const successRate = finished === 0 ? null : Math.round((stats.succeeded / finished) * 1000) / 10;
  const deliveriesLink = (status: DeliveryStatus) => `${href("projects", projectId, "deliveries")}?status=${status}`;

  return (
    <>
      <div className="stats">
        {tiles.map((tile) => (
          <Stat
            key={tile.status}
            label={tile.label}
            icon={tile.icon}
            value={stats[tile.status]}
            share={total === 0 ? 0 : stats[tile.status] / total}
            tone={tile.status === "dead_letter" && stats.dead_letter > 0 ? "bad" : tile.status}
            link={deliveriesLink(tile.status)}
          />
        ))}
      </div>

      <div className="overview-grid">
        <div className="card panel-card">
          <div className="card-head">
            <h2>Delivery health</h2>
            <span className="muted">{total.toLocaleString()} deliveries</span>
          </div>
          <div className="health">
            <div className="health-rate">
              <span className="big-number">{successRate === null ? "—" : `${successRate}%`}</span>
              <span className="muted">success rate of finished deliveries</span>
            </div>
            <div className="segments" role="img" aria-label="Share of deliveries by status">
              {tiles
                .filter((tile) => stats[tile.status] > 0)
                .map((tile) => (
                  <span key={tile.status} className={`segment segment-${tile.status}`} style={{ flexGrow: stats[tile.status] }} />
                ))}
              {total === 0 && <span className="segment segment-empty" style={{ flexGrow: 1 }} />}
            </div>
            <ul className="legend">
              {tiles.map((tile) => (
                <li key={tile.status}>
                  <span className={`legend-dot segment-${tile.status}`} />
                  {tile.label}
                  <strong>{stats[tile.status].toLocaleString()}</strong>
                </li>
              ))}
            </ul>
          </div>
        </div>

        <div className="card panel-card">
          <div className="card-head">
            <h2>Queue</h2>
          </div>
          <p className="big-number">{stats.oldest_due_at ? formatRelative(stats.oldest_due_at) : "Empty"}</p>
          <p className="muted">
            {stats.oldest_due_at
              ? "The oldest due delivery has been waiting this long. If it keeps growing, check that a worker is running."
              : "No deliveries are waiting for a worker."}
          </p>
        </div>
      </div>

      <RecentDeliveries projectId={projectId} />
    </>
  );
}

function RecentDeliveries({ projectId }: { projectId: string }) {
  const api = useApi();
  const [state, reload] = useAsync((signal) => api.listDeliveries(projectId, { limit: 6 }, signal), [api, projectId]);
  return (
    <div className="card panel-card">
      <div className="card-head">
        <h2>Recent deliveries</h2>
        <a className="link-arrow" href={href("projects", projectId, "deliveries")}>
          View all
          <Icon name="arrow" size={14} />
        </a>
      </div>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" && (
        <DeliveryTable projectId={projectId} deliveries={state.data.deliveries} emptyText="Deliveries appear here once events arrive for enabled endpoints." />
      )}
    </div>
  );
}

function Stat({ label, icon, value, share, tone, link }: { label: string; icon: IconName; value: number; share: number; tone: string; link: string }) {
  return (
    <a className={`stat stat-${tone}`} href={link}>
      <span className="stat-top">
        <span className="stat-icon">
          <Icon name={icon} size={16} />
        </span>
        <span className="stat-label">{label}</span>
        <Icon name="arrow" size={14} />
      </span>
      <span className="stat-value">{value.toLocaleString()}</span>
      <span className="meter" aria-hidden="true">
        <span style={{ width: `${Math.max(share * 100, value > 0 ? 4 : 0)}%` }} />
      </span>
    </a>
  );
}
