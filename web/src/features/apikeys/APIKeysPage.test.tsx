import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { APIKeysPage } from "./APIKeysPage";
import { fakeFetch, ids, renderWithApi } from "../../test/fakeApi";

const keyId = "66666666-6666-4666-8666-666666666666";
const apiKey = {
  id: keyId,
  project_id: ids.project,
  name: "billing-service",
  prefix: "hlk_ab12",
  revoked: false,
  created_at: "2026-10-17T09:00:00Z",
};
const rawKey = "hlk_ab12" + "c".repeat(60);

afterEach(() => {
  vi.restoreAllMocks();
});

describe("APIKeysPage", () => {
  it("generates a key and shows the raw key once", async () => {
    let created = false;
    const { fetch, calls } = fakeFetch({
      [`GET /v1/projects/${ids.project}/api-keys`]: () => ({ body: { api_keys: created ? [apiKey] : [] } }),
      [`POST /v1/projects/${ids.project}/api-keys`]: () => {
        created = true;
        return { status: 201, body: { ...apiKey, raw_key: rawKey } };
      },
    });
    renderWithApi(<APIKeysPage projectId={ids.project} />, fetch);

    expect(await screen.findByText("No API keys yet")).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText("Key name"), "  billing-service ");
    await userEvent.click(screen.getByRole("button", { name: "Generate API key" }));

    expect(await screen.findByText(rawKey)).toBeInTheDocument();
    expect(await screen.findByText("hlk_ab12…")).toBeInTheDocument();
    expect(calls.find((c) => c.method === "POST")?.body).toEqual({ name: "billing-service" });
    await userEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(screen.queryByText(rawKey)).not.toBeInTheDocument();
  });

  it("revokes a key only after confirmation", async () => {
    let revoked = false;
    const { fetch, calls } = fakeFetch({
      [`GET /v1/projects/${ids.project}/api-keys`]: () => ({ body: { api_keys: [{ ...apiKey, revoked }] } }),
      [`POST /v1/api-keys/${keyId}/revoke`]: () => {
        revoked = true;
        return { body: { ...apiKey, revoked } };
      },
    });
    const confirm = vi.spyOn(window, "confirm").mockReturnValueOnce(false).mockReturnValueOnce(true);
    renderWithApi(<APIKeysPage projectId={ids.project} />, fetch);

    await userEvent.click(await screen.findByRole("button", { name: "Revoke" }));
    expect(calls.some((c) => c.method === "POST")).toBe(false);

    await userEvent.click(screen.getByRole("button", { name: "Revoke" }));
    expect(await screen.findByText("revoked")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Revoke" })).not.toBeInTheDocument();
    expect(confirm).toHaveBeenCalledTimes(2);
  });

  it("shows a validation error from the API", async () => {
    const { fetch } = fakeFetch({
      [`GET /v1/projects/${ids.project}/api-keys`]: () => ({ body: { api_keys: [] } }),
      [`POST /v1/projects/${ids.project}/api-keys`]: () => ({ status: 400, body: { error: { code: "invalid_input", message: "invalid input" } } }),
    });
    renderWithApi(<APIKeysPage projectId={ids.project} />, fetch);
    await userEvent.type(await screen.findByLabelText("Key name"), "x");
    await userEvent.click(screen.getByRole("button", { name: "Generate API key" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Invalid input");
  });
});
