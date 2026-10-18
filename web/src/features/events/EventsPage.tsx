import { useState, type FormEvent } from "react";
import { useApi } from "../../shared/api/context";
import { formatTime, shortId } from "../../shared/format";
import { t } from "../../shared/i18n";
import { readListQuery, updateListQuery } from "../../shared/listQuery";
import { href } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { Icon } from "../../shared/ui/Icon";
import { PageHeader } from "../../shared/ui/PageHeader";
import { Pager } from "../../shared/ui/Pager";
import { Empty, ErrorState, Loading } from "../../shared/ui/States";

export function EventsPage({ projectId, query }: { projectId: string; query: URLSearchParams }) {
  const api = useApi();
  const { offset, limit } = readListQuery(query);
  const eventType = query.get("type") ?? "";
  const [typeInput, setTypeInput] = useState(eventType);
  const [state, reload] = useAsync(
    (signal) => api.listEvents(projectId, { limit, offset, event_type: eventType || undefined }, signal),
    [api, projectId, offset, limit, eventType],
  );
  const base = href("projects", projectId, "events");
  const update = (change: Record<string, string | undefined>) => updateListQuery(base, query, change);

  function applyFilter(event: FormEvent) {
    event.preventDefault();
    update({ type: typeInput.trim() });
  }

  return (
    <section>
      <PageHeader
        title={t("nav.events")}
        subtitle={t("events.subtitle")}
        actions={
          <button type="button" className="button" onClick={reload}>
            <Icon name="refresh" size={15} />
            {t("common.refresh")}
          </button>
        }
      />
      <form className="toolbar" onSubmit={applyFilter} role="search">
        <div className="search">
          <Icon name="search" size={16} />
          <label htmlFor="event-type" className="sr-only">
            {t("events.filterLabel")}
          </label>
          <input id="event-type" placeholder={t("events.filterPlaceholder")} value={typeInput} onChange={(e) => setTypeInput(e.target.value)} />
        </div>
        <button type="submit" className="button">
          {t("common.filter")}
        </button>
        {eventType && (
          <button
            type="button"
            className="button button-ghost"
            onClick={() => {
              setTypeInput("");
              update({ type: undefined });
            }}
          >
            {t("common.clearFilters")}
          </button>
        )}
      </form>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" &&
        (state.data.events.length === 0 && offset === 0 ? (
          <Empty title={eventType ? t("events.noneOfType", { type: eventType }) : t("events.none")}>{!eventType && t("events.noneHint")}</Empty>
        ) : (
          <div className="panel panel-flush">
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>{t("events.col.event")}</th>
                    <th>{t("events.col.type")}</th>
                    <th>{t("events.col.idempotency")}</th>
                    <th>{t("events.col.received")}</th>
                  </tr>
                </thead>
                <tbody>
                  {state.data.events.map((event) => (
                    <tr key={event.id}>
                      <td>
                        <a href={href("projects", projectId, "events", event.id)}>
                          <code>{shortId(event.id)}</code>
                        </a>
                      </td>
                      <td>
                        <code>{event.event_type}</code>
                      </td>
                      <td className="muted">{event.idempotency_key ?? "—"}</td>
                      <td>{formatTime(event.created_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <Pager
              offset={offset}
              limit={limit}
              count={state.data.events.length}
              nextOffset={state.data.next_offset}
              onOffsetChange={(o) => update({ offset: String(o) })}
              onLimitChange={(l) => update({ limit: String(l) })}
            />
          </div>
        ))}
    </section>
  );
}
