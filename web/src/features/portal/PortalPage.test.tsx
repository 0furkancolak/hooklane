import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PortalPage } from "./PortalPage";
import { delivery, fakeFetch, ids, page } from "../../test/fakeApi";

const token = "hlp_" + "b".repeat(48);
const endpoint = {
  id: ids.endpoint,
  project_id: ids.project,
  url: "https://acme.example/hook",
  enabled: true,
  customer: "acme",
  created_at: "2026-10-17T09:00:00Z",
  updated_at: "2026-10-17T09:00:00Z",
};
const me = { customer: "acme", project_name: "Shop", expires_at: "2026-10-19T09:00:00Z" };

afterEach(() => vi.unstubAllGlobals());

describe("PortalPage", () => {
  it("shows the customer's endpoints and deliveries using only the portal token", async () => {
    const { fetch, calls } = fakeFetch({
      "GET /v1/portal/me": () => ({ body: me }),
      "GET /v1/portal/endpoints": () => ({ body: page("endpoints", [endpoint]) }),
      "GET /v1/portal/deliveries": () => ({ body: page("deliveries", [delivery({ status: "succeeded", last_response_status: 200 })]) }),
    });
    vi.stubGlobal("fetch", fetch);
    render(<PortalPage token={token} />);

    expect(await screen.findByRole("heading", { name: "Webhooks for acme" })).toBeInTheDocument();
    expect(await screen.findByText("https://acme.example/hook")).toBeInTheDocument();
    expect(await screen.findByText(ids.delivery.slice(0, 8))).toBeInTheDocument();
    expect(calls.length).toBeGreaterThan(0);
    expect(calls.every((c) => c.auth === `Bearer ${token}` && c.path.startsWith("/v1/portal/"))).toBe(true);
  });

  it("adds an endpoint and shows its signing secret once", async () => {
    let created = false;
    const { fetch, calls } = fakeFetch({
      "GET /v1/portal/me": () => ({ body: me }),
      "GET /v1/portal/endpoints": () => ({ body: page("endpoints", created ? [endpoint] : []) }),
      "GET /v1/portal/deliveries": () => ({ body: page("deliveries", []) }),
      "POST /v1/portal/endpoints": () => {
        created = true;
        return { status: 201, body: { ...endpoint, secret: "whsec_portal" } };
      },
    });
    vi.stubGlobal("fetch", fetch);
    render(<PortalPage token={token} />);

    await userEvent.type(await screen.findByLabelText("Endpoint URL"), "https://acme.example/hook");
    await userEvent.click(screen.getByRole("button", { name: "Add endpoint" }));

    expect(await screen.findByText("whsec_portal")).toBeInTheDocument();
    expect(calls.find((c) => c.method === "POST")?.body).toEqual({ url: "https://acme.example/hook" });
  });

  it("explains an expired link", async () => {
    const { fetch } = fakeFetch({
      "GET /v1/portal/me": () => ({ status: 401, body: { error: { code: "unauthorized", message: "unauthorized" } } }),
      "GET /v1/portal/endpoints": () => ({ status: 401, body: { error: { code: "unauthorized", message: "unauthorized" } } }),
      "GET /v1/portal/deliveries": () => ({ status: 401, body: { error: { code: "unauthorized", message: "unauthorized" } } }),
    });
    vi.stubGlobal("fetch", fetch);
    render(<PortalPage token={token} />);

    expect(await screen.findByText("This link is no longer valid")).toBeInTheDocument();
  });
});
