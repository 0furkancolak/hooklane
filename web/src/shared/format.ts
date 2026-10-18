import { locale, t } from "./i18n";
import type { MessageKey } from "./i18n/en";

export function formatTime(value: string | null | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString(locale(), { dateStyle: "medium", timeStyle: "medium" });
}

export function formatDate(value: string | null | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString(locale(), { dateStyle: "medium" });
}

export function formatNumber(value: number): string {
  return value.toLocaleString(locale());
}

export function formatRelative(value: string | null | undefined, now = Date.now()): string {
  if (!value) return "—";
  const seconds = Math.round((new Date(value).getTime() - now) / 1000);
  const abs = Math.abs(seconds);
  const [amount, unit]: [number, Intl.RelativeTimeFormatUnit] =
    abs < 60 ? [seconds, "second"] : abs < 3600 ? [Math.round(seconds / 60), "minute"] : abs < 86400 ? [Math.round(seconds / 3600), "hour"] : [Math.round(seconds / 86400), "day"];
  return new Intl.RelativeTimeFormat(locale(), { numeric: "auto", style: "short" }).format(amount, unit);
}

export function shortId(id: string): string {
  return id.slice(0, 8);
}

export function statusLabel(status: string): string {
  return t(`status.${status}` as MessageKey);
}
