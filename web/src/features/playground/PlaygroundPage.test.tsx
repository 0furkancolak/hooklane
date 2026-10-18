import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PlaygroundPage } from "./PlaygroundPage";
import { delivery, fakeFetch, ids, page, renderWithApi } from "../../test/fakeApi";

const endpoint = { id: ids.endpoint, project_id: ids.project, url: "https://hooks.example.com/in", enabled: true, created_at: "2026-10-17T09:00:00Z", updated_at: "2026-10-17T09:00:00Z" };
const event = { id: ids.event, project_id: ids.project, event_type: "invoice.paid", payload: {}, idempotency_key: null, created_at: "2026-10-17T10:00:00Z" };

beforeEach(() => {
  localStorage.clear();
  // The built-in receiver exists only in development; here the API is not.
  vi.stubGlobal("fetch", async () => new Response(null, { status: 404 }));
});

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

function routes(overrides: Parameters<typeof fakeFetch>[0] = {}) {
  return fakeFetch({
    [`GET /v1/projects/${ids.project}/endpoints`]: () => ({ body: page("endpoints", []) }),
    [`POST /v1/projects/${ids.project}/endpoints`]: () => ({ status: 201, body: { ...endpoint, secret: "whsec_x" } }),
    [`POST /v1/projects/${ids.project}/events`]: () => ({ status: 202, body: { ...event, delivery_count: 1 } }),
    [`GET /v1/events/${ids.event}/deliveries`]: () => ({ body: { deliveries: [delivery({ status: "succeeded", attempt_count: 1, last_response_status: 200, last_error: null })] } }),
    [`GET /v1/deliveries/${ids.delivery}/attempts`]: () => ({
      body: { attempts: [{ id: "a1", attempt_number: 1, status_code: 200, response_body: null, error: null, duration_ms: 12, attempted_at: "2026-10-17T10:00:01Z" }] },
    }),
    ...overrides,
  });
}

describe("PlaygroundPage", () => {
  it("sends an event to the chosen URL and shows the delivery result", async () => {
    const { fetch, calls } = routes();
    renderWithApi(<PlaygroundPage projectId={ids.project} />, fetch);

    await userEvent.type(await screen.findByLabelText("Receiver URL"), "https://hooks.example.com/in");
    await userEvent.click(screen.getByRole("button", { name: "Send test event" }));

    expect(await screen.findByText(/was stored as/)).toBeInTheDocument();
    expect(await screen.findByText("succeeded")).toBeInTheDocument();
    expect(await screen.findByText(/Attempt 1:/)).toBeInTheDocument();
    expect(calls.find((c) => c.method === "POST" && c.path.endsWith("/endpoints"))?.body).toEqual({ url: "https://hooks.example.com/in" });
    expect(calls.find((c) => c.method === "POST" && c.path.endsWith("/events"))?.body).toMatchObject({ event_type: "invoice.paid" });
  });

  it("refuses a payload that is not valid JSON without calling the API", async () => {
    const { fetch, calls } = routes();
    renderWithApi(<PlaygroundPage projectId={ids.project} />, fetch);

    await userEvent.type(await screen.findByLabelText("Receiver URL"), "https://hooks.example.com/in");
    const payload = screen.getByLabelText("Payload (JSON)");
    await userEvent.clear(payload);
    await userEvent.type(payload, "not json");
    await userEvent.click(screen.getByRole("button", { name: "Send test event" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("not valid JSON");
    expect(calls.filter((c) => c.method === "POST")).toHaveLength(0);
  });

  it("warns when the project already has other enabled endpoints", async () => {
    const { fetch } = routes({ [`GET /v1/projects/${ids.project}/endpoints`]: () => ({ body: page("endpoints", [endpoint]) }) });
    renderWithApi(<PlaygroundPage projectId={ids.project} />, fetch);

    expect(await screen.findByRole("note")).toHaveTextContent("1 other enabled endpoint");
  });

  it("disables the built-in receiver outside development", async () => {
    const { fetch } = routes();
    renderWithApi(<PlaygroundPage projectId={ids.project} />, fetch);
    await screen.findByLabelText("Receiver URL");
    expect(screen.getByRole("radio", { name: /Built-in receiver/ })).toBeDisabled();
  });
});
