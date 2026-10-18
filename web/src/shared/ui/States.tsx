import type { ReactNode } from "react";
import { describeError } from "../errors";
import { t } from "../i18n";

export function Loading({ label }: { label?: string }) {
  return (
    <p className="state" role="status" aria-live="polite">
      {label ?? t("common.loading")}
    </p>
  );
}

export function ErrorState({ error, onRetry }: { error: Error; onRetry?: () => void }) {
  return (
    <div className="state state-error" role="alert">
      <p>{describeError(error)}</p>
      {onRetry && (
        <button type="button" className="button" onClick={onRetry}>
          {t("common.retry")}
        </button>
      )}
    </div>
  );
}

export function Empty({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="state state-empty">
      <p className="state-title">{title}</p>
      {children && <div className="hint">{children}</div>}
    </div>
  );
}
