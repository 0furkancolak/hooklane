import { navigate } from "./router";
import { defaultPageSize, pageSizes } from "./pageSize";

/**
 * Filters, page size and position of a list live in the URL query, so a view
 * can be bookmarked, shared and survives a reload. Changing any filter goes
 * back to the first page.
 */
export function readListQuery(query: URLSearchParams) {
  const offset = Math.max(0, Number(query.get("offset") ?? 0) || 0);
  const requested = Number(query.get("limit"));
  const limit = (pageSizes as readonly number[]).includes(requested) ? requested : defaultPageSize;
  return { offset: offset - (offset % limit), limit };
}

export function updateListQuery(basePath: string, query: URLSearchParams, change: Record<string, string | undefined>) {
  const next = new URLSearchParams(query);
  for (const [key, value] of Object.entries(change)) {
    if (value === undefined || value === "") next.delete(key);
    else next.set(key, value);
  }
  if (!("offset" in change)) next.delete("offset");
  const search = next.toString();
  navigate(basePath + (search ? `?${search}` : ""));
}
