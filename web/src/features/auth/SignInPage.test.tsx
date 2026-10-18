import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { LangProvider } from "../../shared/i18n/LangProvider";
import { SignInPage } from "./SignInPage";

const token = "hls_" + "a".repeat(48);

function stubFetch(handler: (path: string, init?: RequestInit) => Response) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => handler(String(input), init)),
  );
}

beforeEach(() => localStorage.setItem("hooklane.lang", "en"));
afterEach(() => vi.unstubAllGlobals());

function json(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status });
}

test("signs in with email and password", async () => {
  const onSignedIn = vi.fn();
  stubFetch((path) => (path === "/v1/auth/signin" ? json(200, { token }) : json(404, {})));
  render(
    <LangProvider>
      <SignInPage onSignedIn={onSignedIn} />
    </LangProvider>,
  );
  await userEvent.type(screen.getByLabelText("Email"), "ada@example.com");
  await userEvent.type(screen.getByLabelText("Password"), "correct horse battery");
  await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
  await waitFor(() => expect(onSignedIn).toHaveBeenCalledWith(token));
});

test("shows a clear message for wrong credentials", async () => {
  stubFetch((path) =>
    path === "/v1/auth/signin" ? json(401, { error: { code: "invalid_credentials", message: "invalid email or password" } }) : json(404, {}),
  );
  render(
    <LangProvider>
      <SignInPage onSignedIn={vi.fn()} />
    </LangProvider>,
  );
  await userEvent.type(screen.getByLabelText("Email"), "ada@example.com");
  await userEvent.type(screen.getByLabelText("Password"), "wrong password");
  await userEvent.click(screen.getByRole("button", { name: "Sign in" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Wrong email or password.");
});

test("creates an account", async () => {
  const onSignedIn = vi.fn();
  let sent: unknown;
  stubFetch((path, init) => {
    if (path === "/v1/auth/signup") {
      sent = JSON.parse(String(init?.body));
      return json(201, { token });
    }
    return json(404, {});
  });
  render(
    <LangProvider>
      <SignInPage onSignedIn={onSignedIn} />
    </LangProvider>,
  );
  await userEvent.click(screen.getByRole("button", { name: "Create an account" }));
  await userEvent.type(screen.getByLabelText("Email"), "ada@example.com");
  await userEvent.type(screen.getByLabelText("Password"), "correct horse battery");
  await userEvent.type(screen.getByLabelText("Organization name (optional)"), "Acme");
  await userEvent.click(screen.getByRole("button", { name: "Create account" }));
  await waitFor(() => expect(onSignedIn).toHaveBeenCalledWith(token));
  expect(sent).toEqual({ email: "ada@example.com", password: "correct horse battery", organization_name: "Acme" });
});
