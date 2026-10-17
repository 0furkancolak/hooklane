import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { EndpointsPage } from "./EndpointsPage";
import { fakeFetch, ids, page, renderWithApi } from "../../test/fakeApi";

const endpoint = {
  id: ids.endpoint,
  project_id: ids.project,
  url: "https://hooks.example.com/in",
  enabled: true,
  created_at: "2026-10-17T09:00:00Z",
  updated_at: "2026-10-17T09:00:00Z",
};

describe("EndpointsPage", () => {
  it("shows the empty state, then the signing secret once after creating", async () => {
    let created = false;
    const { fetch, calls } = fakeFetch({
      [`GET /v1/projects/${ids.project}/endpoints`]: () => ({ body: page("endpoints", created ? [endpoint] : []) }),
      [`POST /v1/projects/${ids.project}/endpoints`]: () => {
        created = true;
        return { status: 201, body: { ...endpoint, secret: "whsec_test" } };
      },
    });
    renderWithApi(<EndpointsPage projectId={ids.project} />, fetch);

    expect(await screen.findByText("No endpoints yet")).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText("Endpoint URL"), "https://hooks.example.com/in");
    await userEvent.click(screen.getByRole("button", { name: "Add endpoint" }));

    expect(await screen.findByText("whsec_test")).toBeInTheDocument();
    expect(await screen.findByText("https://hooks.example.com/in", { selector: "code.url" })).toBeInTheDocument();
    expect(calls.find((c) => c.method === "POST")?.body).toEqual({ url: "https://hooks.example.com/in" });
    await userEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(screen.queryByText("whsec_test")).not.toBeInTheDocument();
  });

  it("explains a blocked destination", async () => {
    const { fetch } = fakeFetch({
      [`GET /v1/projects/${ids.project}/endpoints`]: () => ({ body: page("endpoints", []) }),
      [`POST /v1/projects/${ids.project}/endpoints`]: () => ({
        status: 400,
        body: { error: { code: "destination_not_allowed", message: "endpoint URL points to an address that is not allowed" } },
      }),
    });
    renderWithApi(<EndpointsPage projectId={ids.project} />, fetch);
    await userEvent.type(await screen.findByLabelText("Endpoint URL"), "https://169.254.169.254/");
    await userEvent.click(screen.getByRole("button", { name: "Add endpoint" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("private, internal or reserved address");
  });

  it("toggles an endpoint and reveals its secret on request", async () => {
    let enabled = true;
    const { fetch, calls } = fakeFetch({
      [`GET /v1/projects/${ids.project}/endpoints`]: () => ({ body: page("endpoints", [{ ...endpoint, enabled }]) }),
      [`PATCH /v1/endpoints/${ids.endpoint}`]: ({ body }) => {
        enabled = (body as { enabled: boolean }).enabled;
        return { body: { ...endpoint, enabled } };
      },
      [`GET /v1/endpoints/${ids.endpoint}/secret`]: () => ({ body: { secret: "whsec_revealed" } }),
    });
    renderWithApi(<EndpointsPage projectId={ids.project} />, fetch);

    await userEvent.click(await screen.findByRole("button", { name: "Disable" }));
    expect(await screen.findByRole("button", { name: "Enable" })).toBeInTheDocument();
    expect(calls.find((c) => c.method === "PATCH")?.body).toEqual({ enabled: false });

    expect(screen.queryByText("whsec_revealed")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Show secret" }));
    expect(await screen.findByText("whsec_revealed")).toBeInTheDocument();
  });
});
