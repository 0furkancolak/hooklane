import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { DeliveryDetailPage } from "./DeliveryDetailPage";
import { delivery, fakeFetch, ids, renderWithApi } from "../../test/fakeApi";

const attempts = {
  attempts: [
    { id: "a1", attempt_number: 1, status_code: null, response_body: null, error: "dial tcp: connection refused", duration_ms: 3, attempted_at: "2026-10-17T10:00:00Z" },
    { id: "a2", attempt_number: 2, status_code: 503, response_body: "upstream down", error: "endpoint responded with status 503", duration_ms: 41, attempted_at: "2026-10-17T10:01:00Z" },
  ],
};

describe("DeliveryDetailPage", () => {
  it("shows attempt history with status, error and response excerpt", async () => {
    const { fetch } = fakeFetch({
      [`GET /v1/deliveries/${ids.delivery}`]: () => ({ body: delivery() }),
      [`GET /v1/deliveries/${ids.delivery}/attempts`]: () => ({ body: attempts }),
    });
    renderWithApi(<DeliveryDetailPage projectId={ids.project} deliveryId={ids.delivery} />, fetch);

    expect(await screen.findByText("#2")).toBeInTheDocument();
    expect(screen.getByText("Dead letter")).toBeInTheDocument();
    expect(screen.getByText("dial tcp: connection refused")).toBeInTheDocument();
    expect(screen.getByText("No response")).toBeInTheDocument();
    expect(screen.getByText("503")).toBeInTheDocument();
    expect(screen.getByText("upstream down")).toBeInTheDocument();
  });

  it("asks for confirmation, replays and opens the new delivery", async () => {
    const { fetch, calls } = fakeFetch({
      [`GET /v1/deliveries/${ids.delivery}`]: () => ({ body: delivery() }),
      [`GET /v1/deliveries/${ids.delivery}/attempts`]: () => ({ body: attempts }),
      [`POST /v1/deliveries/${ids.delivery}/replay`]: () => ({ status: 201, body: delivery({ id: ids.replay, status: "pending", replay_of: ids.delivery }) }),
    });
    renderWithApi(<DeliveryDetailPage projectId={ids.project} deliveryId={ids.delivery} />, fetch);

    await userEvent.click(await screen.findByRole("button", { name: "Replay" }));
    expect(calls.some((c) => c.method === "POST")).toBe(false);
    await userEvent.click(screen.getByRole("button", { name: "Confirm replay" }));

    await waitFor(() => expect(window.location.hash).toBe(`#/projects/${ids.project}/deliveries/${ids.replay}`));
    expect(calls.filter((c) => c.method === "POST")).toHaveLength(1);
  });

  it("explains a conflicting replay", async () => {
    const { fetch } = fakeFetch({
      [`GET /v1/deliveries/${ids.delivery}`]: () => ({ body: delivery() }),
      [`GET /v1/deliveries/${ids.delivery}/attempts`]: () => ({ body: attempts }),
      [`POST /v1/deliveries/${ids.delivery}/replay`]: () => ({
        status: 409,
        body: { error: { code: "replay_in_progress", message: "an earlier replay of this delivery has not finished" } },
      }),
    });
    renderWithApi(<DeliveryDetailPage projectId={ids.project} deliveryId={ids.delivery} />, fetch);

    await userEvent.click(await screen.findByRole("button", { name: "Replay" }));
    await userEvent.click(screen.getByRole("button", { name: "Confirm replay" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("An earlier replay of this delivery has not finished");
  });

  it("does not offer replay while a delivery is still in progress", async () => {
    const { fetch } = fakeFetch({
      [`GET /v1/deliveries/${ids.delivery}`]: () => ({ body: delivery({ status: "pending", next_attempt_at: "2026-10-17T10:10:00Z" }) }),
      [`GET /v1/deliveries/${ids.delivery}/attempts`]: () => ({ body: { attempts: [] } }),
    });
    renderWithApi(<DeliveryDetailPage projectId={ids.project} deliveryId={ids.delivery} />, fetch);

    expect(await screen.findByRole("button", { name: "Replay" })).toBeDisabled();
    expect(screen.getByText("No attempts yet")).toBeInTheDocument();
  });

  it("shows a forbidden error for another organization's delivery", async () => {
    const { fetch } = fakeFetch({
      [`GET /v1/deliveries/${ids.delivery}`]: () => ({ status: 403, body: { error: { code: "forbidden", message: "forbidden" } } }),
      [`GET /v1/deliveries/${ids.delivery}/attempts`]: () => ({ status: 403, body: { error: { code: "forbidden", message: "forbidden" } } }),
    });
    renderWithApi(<DeliveryDetailPage projectId={ids.project} deliveryId={ids.delivery} />, fetch);
    expect(await screen.findByRole("alert")).toHaveTextContent("does not own");
  });
});
