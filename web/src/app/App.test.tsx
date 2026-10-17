import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { fakeFetch, ids, page, testKey } from "../test/fakeApi";

const project = { id: ids.project, organization_id: "o", name: "Shop", created_at: "2026-10-17T09:00:00Z" };

afterEach(() => vi.unstubAllGlobals());

describe("App", () => {
  it("rejects malformed keys without calling the API", async () => {
    const { fetch, calls } = fakeFetch({});
    vi.stubGlobal("fetch", fetch);
    render(<App />);
    await userEvent.type(screen.getByLabelText("API key"), "not-a-key");
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(screen.getByRole("alert")).toHaveTextContent("does not look like");
    expect(calls).toHaveLength(0);
  });

  it("signs in, keeps the key in session storage only, and lists projects", async () => {
    const { fetch, calls } = fakeFetch({ "GET /v1/projects": () => ({ body: page("projects", [project]) }) });
    vi.stubGlobal("fetch", fetch);
    render(<App />);
    await userEvent.type(screen.getByLabelText("API key"), testKey);
    await userEvent.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findByRole("link", { name: /Shop/ })).toBeInTheDocument();
    expect(sessionStorage.getItem("hooklane.apiKey")).toBe(testKey);
    expect(localStorage.length).toBe(0);
    expect(calls.every((c) => c.auth === `Bearer ${testKey}`)).toBe(true);
  });

  it("returns to sign-in with a notice when the key is revoked mid-session", async () => {
    sessionStorage.setItem("hooklane.apiKey", testKey);
    const { fetch } = fakeFetch({
      "GET /v1/projects": () => ({ status: 401, body: { error: { code: "revoked", message: "api key revoked" } } }),
    });
    vi.stubGlobal("fetch", fetch);
    render(<App />);
    expect(await screen.findByText(/sign in again/)).toBeInTheDocument();
    expect(screen.getByLabelText("API key")).toBeInTheDocument();
    expect(sessionStorage.getItem("hooklane.apiKey")).toBeNull();
  });
});
