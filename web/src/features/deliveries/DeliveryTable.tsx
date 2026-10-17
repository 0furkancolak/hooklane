import type { Delivery } from "../../shared/api/types";
import { href } from "../../shared/router";
import { formatRelative, formatTime, shortId } from "../../shared/format";
import { Empty } from "../../shared/ui/States";
import { HttpStatus, StatusBadge } from "../../shared/ui/StatusBadge";

export function DeliveryTable({ projectId, deliveries, emptyText }: { projectId: string; deliveries: Delivery[]; emptyText: string }) {
  if (deliveries.length === 0) {
    return <Empty title="No deliveries">{emptyText}</Empty>;
  }
  return (
    <div className="table-wrap">
      <table className="table">
      <thead>
        <tr>
          <th>Delivery</th>
          <th>Status</th>
          <th>Attempts</th>
          <th>Last response</th>
          <th>Next attempt</th>
          <th>Created</th>
        </tr>
      </thead>
      <tbody>
        {deliveries.map((d) => (
          <tr key={d.id}>
            <td>
              <a href={href("projects", projectId, "deliveries", d.id)}>
                <code>{shortId(d.id)}</code>
              </a>
              {d.replay_of && <span className="tag">replay</span>}
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
