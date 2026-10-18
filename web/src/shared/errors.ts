import { ApiError } from "./api/client";
import { t } from "./i18n";

export function describeError(error: Error): string {
  if (error instanceof ApiError) {
    switch (error.status) {
      case 0:
        return t("error.network");
      case 403:
        return t("error.forbidden");
      case 404:
        return t("error.notFound");
      case 429:
        return t("error.rateLimited");
      default:
        return error.message.charAt(0).toUpperCase() + error.message.slice(1);
    }
  }
  return t("error.generic");
}
