import { ApiError } from "./api/client";

export function describeError(error: Error): string {
  if (error instanceof ApiError) {
    switch (error.status) {
      case 0:
        return "Could not reach the Hooklane API. Check that it is running.";
      case 403:
        return "This API key's organization does not own that resource.";
      case 429:
        return "Too many requests. Wait a moment and try again.";
      case 404:
        return "Not found.";
      default:
        return error.message.charAt(0).toUpperCase() + error.message.slice(1);
    }
  }
  return "Something went wrong.";
}
