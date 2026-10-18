import { useEffect, useState } from "react";

// Hash routes keep the dashboard deployable as static files behind any server.
export type Route =
  | { name: "projects" }
  | { name: "portal"; token: string }
  | { name: "overview"; projectId: string }
  | { name: "endpoints"; projectId: string; query: URLSearchParams }
  | { name: "playground"; projectId: string }
  | { name: "api_keys"; projectId: string }
  | { name: "events"; projectId: string; query: URLSearchParams }
  | { name: "event"; projectId: string; eventId: string }
  | { name: "deliveries"; projectId: string; query: URLSearchParams }
  | { name: "delivery"; projectId: string; deliveryId: string }
  | { name: "not_found" };

export function parseRoute(hash: string): Route {
  const [path, search = ""] = hash.replace(/^#/, "").split("?", 2);
  const parts = path.split("/").filter(Boolean).map(decodeURIComponent);
  if (parts.length === 0 || (parts.length === 1 && parts[0] === "projects")) {
    return { name: "projects" };
  }
  if (parts[0] === "portal" && parts.length === 2) {
    return { name: "portal", token: parts[1] };
  }
  if (parts[0] !== "projects" || !parts[1]) {
    return { name: "not_found" };
  }
  const projectId = parts[1];
  const [section, id, extra] = parts.slice(2);
  if (extra !== undefined) {
    return { name: "not_found" };
  }
  switch (section) {
    case undefined:
      return { name: "overview", projectId };
    case "endpoints":
      return id === undefined ? { name: "endpoints", projectId, query: new URLSearchParams(search) } : { name: "not_found" };
    case "playground":
      return id === undefined ? { name: "playground", projectId } : { name: "not_found" };
    case "api-keys":
      return id === undefined ? { name: "api_keys", projectId } : { name: "not_found" };
    case "events":
      return id === undefined ? { name: "events", projectId, query: new URLSearchParams(search) } : { name: "event", projectId, eventId: id };
    case "deliveries":
      return id === undefined
        ? { name: "deliveries", projectId, query: new URLSearchParams(search) }
        : { name: "delivery", projectId, deliveryId: id };
    default:
      return { name: "not_found" };
  }
}

export function href(...parts: string[]): string {
  return `#/${parts.map(encodeURIComponent).join("/")}`;
}

export function useRoute(): Route {
  const [route, setRoute] = useState(() => parseRoute(window.location.hash));
  useEffect(() => {
    const onChange = () => setRoute(parseRoute(window.location.hash));
    window.addEventListener("hashchange", onChange);
    return () => window.removeEventListener("hashchange", onChange);
  }, []);
  return route;
}

export function navigate(hash: string): void {
  window.location.hash = hash;
}
