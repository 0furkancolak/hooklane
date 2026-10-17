export function Pager({
  offset,
  limit,
  nextOffset,
  onChange,
}: {
  offset: number;
  limit: number;
  nextOffset: number | null;
  onChange: (offset: number) => void;
}) {
  if (offset === 0 && nextOffset === null) return null;
  return (
    <nav className="pager" aria-label="Pagination">
      <button type="button" className="button" disabled={offset === 0} onClick={() => onChange(Math.max(0, offset - limit))}>
        Newer
      </button>
      <span className="muted">
        {offset + 1}–{offset + limit}
      </span>
      <button type="button" className="button" disabled={nextOffset === null} onClick={() => nextOffset !== null && onChange(nextOffset)}>
        Older
      </button>
    </nav>
  );
}
