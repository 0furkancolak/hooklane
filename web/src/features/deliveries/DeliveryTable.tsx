import type { Delivery } from "../../shared/api/types";
import { formatRelative, formatTime, shortId } from "../../shared/format";
import { t } from "../../shared/i18n";
import { href } from "../../shared/router";
import { Empty } from "../../shared/ui/States";
import { HttpStatus, StatusBadge } from "../../shared/ui/StatusBadge";

/** The rows of a delivery list. Wrap it in a .panel-flush to add a pager footer. */
export function DeliveryTable({ projectId, deliveries, emptyText, flush = false }: { projectId: string; deliveries: Delivery[]; emptyText: string; flush?: boolean }) {
  if (deliveries.length === 0) {
    return <Empty title={t("deliveries.none")}>{emptyText}</Empty>;
  }
  return (
    <div className={`table-wrap ${flush ? "table-wrap-flush" : ""}`}>
      <table className="table">
        <thead>
          <tr>
            <th>{t("deliveries.col.delivery")}</th>
            <th>{t("deliveries.col.status")}</th>
            <th>{t("deliveries.col.attempts")}</th>
            <th>{t("deliveries.col.response")}</th>
            <th>{t("deliveries.col.next")}</th>
            <th>{t("deliveries.col.created")}</th>
          </tr>
        </thead>
        <tbody>
          {deliveries.map((d) => (
            <tr key={d.id}>
              <td>
                <a href={href("projects", projectId, "deliveries", d.id)}>
                  <code>{shortId(d.id)}</code>
                </a>
                {d.replay_of && <span className="tag">{t("deliveries.replay")}</span>}
              </td>
              <td>
                <StatusBadge status={d.status} />
              </td>
              <td>{d.attempt_count}</td>
              <td>{d.attempt_count === 0 ? <span className="muted">—</span> : <HttpStatus code={d.last_response_status} />}</td>
              <td>{d.next_attempt_at ? formatRelative(d.next_attempt_at) : "—"}</td>
              <td>{formatTime(d.created_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
