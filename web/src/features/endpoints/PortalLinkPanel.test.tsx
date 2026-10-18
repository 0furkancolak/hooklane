import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { PortalLinkPanel } from "./PortalLinkPanel";
import { fakeFetch, ids, renderWithApi } from "../../test/fakeApi";

describe("PortalLinkPanel", () => {
  it("creates a link for a customer and shows the portal URL", async () => {
    const token = "hlp_" + "c".repeat(48);
    const { fetch, calls } = fakeFetch({
      [`POST /v1/projects/${ids.project}/portal-links`]: () => ({ status: 201, body: { token, customer: "acme", expires_at: "2026-10-19T09:00:00Z", path: `/#/portal/${token}` } }),
    });
    renderWithApi(<PortalLinkPanel projectId={ids.project} />, fetch);

    await userEvent.type(screen.getByLabelText("Customer"), "acme");
    await userEvent.selectOptions(screen.getByLabelText("Link validity"), "604800");
    await userEvent.click(screen.getByRole("button", { name: "Create link" }));

    expect(await screen.findByText(new RegExp(`/#/portal/${token}$`))).toBeInTheDocument();
    expect(calls[0].body).toEqual({ customer: "acme", ttl_seconds: 604800 });
  });
});
