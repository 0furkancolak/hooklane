import type { DeliveryStats, DeliveryStatus } from "../../shared/api/types";
import { useApi } from "../../shared/api/context";
import { href } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { formatNumber, formatRelative } from "../../shared/format";
import { statusLabel } from "../../shared/format";
import { t } from "../../shared/i18n";
import { PageHeader } from "../../shared/ui/PageHeader";
import { Icon, type IconName } from "../../shared/ui/Icon";
import { ErrorState, Loading } from "../../shared/ui/States";
import { DeliveryTable } from "../deliveries/DeliveryTable";

const tiles: { status: DeliveryStatus; icon: IconName }[] = [
  { status: "pending", icon: "clock" },
  { status: "processing", icon: "loader" },
  { status: "succeeded", icon: "check" },
  { status: "dead_letter", icon: "alert" },
];

export function OverviewPage({ projectId }: { projectId: string }) {
  const api = useApi();
  const [state, reload] = useAsync((signal) => api.getDeliveryStats(projectId, signal), [api, projectId]);

  return (
    <section>
      <PageHeader
        title={t("nav.overview")}
        subtitle={t("overview.subtitle")}
        actions={
          <button type="button" className="button" onClick={reload}>
            <Icon name="refresh" size={15} />
            {t("common.refresh")}
          </button>
        }
      />
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
      {total === 0 && (
        <div className="panel hero">
          <div>
            <h2>{t("overview.empty.title")}</h2>
            <p className="muted">
              {t("overview.empty.text")} <code>POST /v1/projects/{projectId.slice(0, 8)}…/events</code>
            </p>
          </div>
          <a className="button button-primary" href={href("projects", projectId, "playground")}>
            <Icon name="play" size={16} />
            {t("projects.openPlayground")}
          </a>
        </div>
      )}
      <div className="stats">
        {tiles.map((tile) => (
          <Stat
            key={tile.status}
            label={statusLabel(tile.status)}
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
            <h2>{t("overview.health")}</h2>
            <span className="muted">{t("overview.deliveriesCount", { count: formatNumber(total) })}</span>
          </div>
          <div className="health">
            <div className="health-rate">
              <span className="big-number">{successRate === null ? "—" : `${successRate}%`}</span>
              <span className="muted">{t("overview.successRate")}</span>
            </div>
            <div className="segments" role="img" aria-label={t("overview.shareLabel")}>
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
                  {statusLabel(tile.status)}
                  <strong>{formatNumber(stats[tile.status])}</strong>
                </li>
              ))}
            </ul>
          </div>
        </div>

        <div className="card panel-card">
          <div className="card-head">
            <h2>{t("overview.queue")}</h2>
          </div>
          <p className="big-number">{stats.oldest_due_at ? formatRelative(stats.oldest_due_at) : t("overview.queueEmpty")}</p>
          <p className="muted">
            {stats.oldest_due_at
              ? t("overview.queueWaiting")
              : t("overview.queueNone")}
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
        <h2>{t("overview.recent")}</h2>
        <a className="link-arrow" href={href("projects", projectId, "deliveries")}>
          {t("common.viewAll")}
          <Icon name="arrow" size={14} />
        </a>
      </div>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" && (
        <DeliveryTable projectId={projectId} deliveries={state.data.deliveries} emptyText={t("deliveries.emptyHint")} />
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
      <span className="stat-value">{formatNumber(value)}</span>
      <span className="meter" aria-hidden="true">
        <span style={{ width: `${Math.max(share * 100, value > 0 ? 4 : 0)}%` }} />
      </span>
    </a>
  );
}
