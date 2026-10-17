import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { DeliveriesPage } from "./DeliveriesPage";
import { delivery, fakeFetch, ids, page, renderWithApi } from "../../test/fakeApi";

describe("DeliveriesPage", () => {
  it("passes URL filters to the API and pages forward", async () => {
    const { fetch, calls } = fakeFetch({
      [`GET /v1/projects/${ids.project}/deliveries`]: () => ({ body: page("deliveries", [delivery()], 25) }),
    });
    renderWithApi(<DeliveriesPage projectId={ids.project} query={new URLSearchParams("status=dead_letter")} />, fetch);

    expect(await screen.findByText(ids.delivery.slice(0, 8))).toBeInTheDocument();
    expect(calls[0].search).toContain("status=dead_letter");
    await userEvent.click(screen.getByRole("button", { name: "Older" }));
    expect(window.location.hash).toBe(`#/projects/${ids.project}/deliveries?status=dead_letter&offset=25`);
  });

  it("changing the status filter resets paging", async () => {
    const { fetch } = fakeFetch({ [`GET /v1/projects/${ids.project}/deliveries`]: () => ({ body: page("deliveries", []) }) });
    renderWithApi(<DeliveriesPage projectId={ids.project} query={new URLSearchParams("offset=50")} />, fetch);
    await userEvent.click(await screen.findByRole("button", { name: "succeeded" }));
    expect(window.location.hash).toBe(`#/projects/${ids.project}/deliveries?status=succeeded`);
  });

  it("shows an error with retry, then recovers", async () => {
    let fail = true;
    const { fetch } = fakeFetch({
      [`GET /v1/projects/${ids.project}/deliveries`]: () =>
        fail ? { status: 500, body: { error: { code: "internal_error", message: "internal error" } } } : { body: page("deliveries", []) },
    });
    renderWithApi(<DeliveriesPage projectId={ids.project} query={new URLSearchParams()} />, fetch);

    expect(await screen.findByRole("alert")).toHaveTextContent("Internal error");
    fail = false;
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(screen.getByText("No deliveries")).toBeInTheDocument());
  });
});
