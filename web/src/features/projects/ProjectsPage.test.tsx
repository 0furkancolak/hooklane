import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ProjectsPage } from "./ProjectsPage";
import { fakeFetch, ids, page, renderWithApi } from "../../test/fakeApi";

const project = { id: ids.project, organization_id: "o", name: "Shop", created_at: "2026-10-17T09:00:00Z" };

afterEach(() => {
  window.location.hash = "";
  vi.restoreAllMocks();
});

describe("ProjectsPage", () => {
  it("explains what Hooklane is for and offers the playground to a new user", async () => {
    const { fetch } = fakeFetch({ "GET /v1/projects": () => ({ body: page("projects", []) }) });
    renderWithApi(<ProjectsPage />, fetch);

    expect(await screen.findByText("New here? Try it in two minutes")).toBeInTheDocument();
    expect(screen.getByText("What is Hooklane for?")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open the playground" })).toBeInTheDocument();
  });

  it("creates a project and opens it", async () => {
    const { fetch, calls } = fakeFetch({
      "GET /v1/projects": () => ({ body: page("projects", [project]) }),
      "POST /v1/projects": () => ({ status: 201, body: { ...project, name: "Acme" } }),
    });
    renderWithApi(<ProjectsPage />, fetch);

    await userEvent.type(await screen.findByLabelText("Project name"), "  Acme ");
    await userEvent.click(screen.getByRole("button", { name: "Create project" }));

    await vi.waitFor(() => expect(window.location.hash).toBe(`#/projects/${ids.project}`));
    expect(calls.find((c) => c.method === "POST")?.body).toEqual({ name: "Acme" });
  });

  it("creates the playground project from the first-run call to action", async () => {
    const { fetch, calls } = fakeFetch({
      "GET /v1/projects": () => ({ body: page("projects", []) }),
      "POST /v1/projects": () => ({ status: 201, body: { ...project, name: "Playground" } }),
    });
    renderWithApi(<ProjectsPage />, fetch);

    await userEvent.click(await screen.findByRole("button", { name: "Open the playground" }));

    await vi.waitFor(() => expect(window.location.hash).toBe(`#/projects/${ids.project}/playground`));
    expect(calls.find((c) => c.method === "POST")?.body).toEqual({ name: "Playground" });
  });
});
