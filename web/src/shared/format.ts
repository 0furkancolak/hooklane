export function formatTime(value: string | null | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

export function formatRelative(value: string | null | undefined, now = Date.now()): string {
  if (!value) return "—";
  const seconds = Math.round((new Date(value).getTime() - now) / 1000);
  const abs = Math.abs(seconds);
  const [amount, unit] =
    abs < 60 ? [abs, "s"] : abs < 3600 ? [Math.round(abs / 60), "m"] : abs < 86400 ? [Math.round(abs / 3600), "h"] : [Math.round(abs / 86400), "d"];
  return seconds >= 0 ? `in ${amount}${unit}` : `${amount}${unit} ago`;
}

export function shortId(id: string): string {
  return id.slice(0, 8);
}

export function statusLabel(status: string): string {
  return status === "dead_letter" ? "dead letter" : status;
}
