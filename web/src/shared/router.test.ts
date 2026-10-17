import { describe, expect, it } from "vitest";
import { href, parseRoute } from "./router";

describe("parseRoute", () => {
  it.each([
    ["", { name: "projects" }],
    ["#/projects", { name: "projects" }],
    ["#/projects/p1", { name: "overview", projectId: "p1" }],
    ["#/projects/p1/endpoints", { name: "endpoints", projectId: "p1" }],
    ["#/projects/p1/api-keys", { name: "api_keys", projectId: "p1" }],
    ["#/projects/p1/events", { name: "events", projectId: "p1" }],
    ["#/projects/p1/events/e1", { name: "event", projectId: "p1", eventId: "e1" }],
    ["#/projects/p1/deliveries/d1", { name: "delivery", projectId: "p1", deliveryId: "d1" }],
    ["#/projects/p1/endpoints/x", { name: "not_found" }],
    ["#/projects/p1/api-keys/x", { name: "not_found" }],
    ["#/projects/p1/events/e1/extra", { name: "not_found" }],
    ["#/nope", { name: "not_found" }],
  ])("%s", (hash, expected) => {
    expect(parseRoute(hash)).toEqual(expected);
  });

  it("keeps delivery filters from the query string", () => {
    const route = parseRoute("#/projects/p1/deliveries?status=dead_letter&endpoint_id=e9");
    expect(route.name).toBe("deliveries");
    if (route.name === "deliveries") {
      expect(route.query.get("status")).toBe("dead_letter");
      expect(route.query.get("endpoint_id")).toBe("e9");
    }
  });

  it("round-trips encoded segments", () => {
    expect(parseRoute(href("projects", "a b/c"))).toEqual({ name: "overview", projectId: "a b/c" });
  });
});
