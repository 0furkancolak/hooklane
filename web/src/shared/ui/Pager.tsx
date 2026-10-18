import { formatNumber } from "../format";
import { t } from "../i18n";
import { pageSizes } from "../pageSize";

/**
 * Pagination for lists the API pages by offset. The API reports no total, so
 * the footer shows the range on screen and whether another page follows.
 */
export function Pager({
  offset,
  limit,
  count,
  nextOffset,
  onOffsetChange,
  onLimitChange,
}: {
  offset: number;
  limit: number;
  /** Rows on the current page. */
  count: number;
  nextOffset: number | null;
  onOffsetChange: (offset: number) => void;
  onLimitChange: (limit: number) => void;
}) {
  const page = Math.floor(offset / limit) + 1;
  const hasPrevious = offset > 0;
  const hasNext = nextOffset !== null;
  return (
    <nav className="pager" aria-label={t("pager.label")}>
      <span className="pager-range muted">{count === 0 ? t("pager.none") : t("pager.range", { from: formatNumber(offset + 1), to: formatNumber(offset + count) })}</span>
      <label className="pager-size">
        <span className="muted">{t("pager.perPage")}</span>
        <select value={limit} onChange={(e) => onLimitChange(Number(e.target.value))}>
          {pageSizes.map((size) => (
            <option key={size} value={size}>
              {size}
            </option>
          ))}
        </select>
      </label>
      <div className="pager-buttons">
        <button type="button" className="button" disabled={!hasPrevious} onClick={() => onOffsetChange(0)} aria-label={t("pager.first")}>
          «
        </button>
        <button type="button" className="button" disabled={!hasPrevious} onClick={() => onOffsetChange(Math.max(0, offset - limit))}>
          {t("pager.previous")}
        </button>
        <span className="pager-page" aria-current="page">
          {t("pager.page", { page })}
        </span>
        <button type="button" className="button" disabled={!hasNext} onClick={() => nextOffset !== null && onOffsetChange(nextOffset)}>
          {t("pager.next")}
        </button>
      </div>
    </nav>
  );
}
