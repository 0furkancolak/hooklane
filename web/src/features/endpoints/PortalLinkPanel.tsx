import { useState, type FormEvent } from "react";
import { useApi } from "../../shared/api/context";
import type { PortalLink } from "../../shared/api/types";
import { describeError } from "../../shared/errors";
import { formatTime } from "../../shared/format";
import { t } from "../../shared/i18n";
import { CopyButton } from "../../shared/ui/CopyButton";

const ttlOptions = [
  { seconds: 3600, label: "portal.ttl.hour" },
  { seconds: 86400, label: "portal.ttl.day" },
  { seconds: 604800, label: "portal.ttl.week" },
] as const;

/** Creates a link that lets one customer manage their own webhook endpoints. */
export function PortalLinkPanel({ projectId }: { projectId: string }) {
  const api = useApi();
  const [customer, setCustomer] = useState("");
  const [ttl, setTtl] = useState<number>(86400);
  const [link, setLink] = useState<PortalLink | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      setLink(await api.createPortalLink(projectId, customer.trim(), ttl));
    } catch (err) {
      setError(err instanceof Error ? describeError(err) : t("portal.createFailed"));
    } finally {
      setBusy(false);
    }
  }

  const url = link ? `${window.location.origin}${link.path}` : "";

  return (
    <div className="panel">
      <h2>{t("portal.panelTitle")}</h2>
      <p className="muted">{t("portal.panelText")}</p>
      <form className="inline-form" onSubmit={submit}>
        <label htmlFor="portal-customer" className="sr-only">
          {t("portal.customerLabel")}
        </label>
        <input id="portal-customer" required maxLength={128} placeholder={t("portal.customerPlaceholder")} value={customer} onChange={(e) => setCustomer(e.target.value)} />
        <label htmlFor="portal-ttl" className="sr-only">
          {t("portal.ttlLabel")}
        </label>
        <select id="portal-ttl" value={ttl} onChange={(e) => setTtl(Number(e.target.value))}>
          {ttlOptions.map((option) => (
            <option key={option.seconds} value={option.seconds}>
              {t(option.label)}
            </option>
          ))}
        </select>
        <button type="submit" className="button" disabled={busy || customer.trim() === ""}>
          {busy ? t("common.creating") : t("portal.create")}
        </button>
        {error && (
          <p className="field-error" role="alert">
            {error}
          </p>
        )}
      </form>
      {link && (
        <div className="secret-inline" role="status">
          <code className="secret-value">{url}</code>
          <CopyButton value={url} />
          <span className="muted">{t("portal.expires", { date: formatTime(link.expires_at) })}</span>
        </div>
      )}
    </div>
  );
}
