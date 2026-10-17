import { describe, expect, it, vi } from "vitest";
import { ApiClient, ApiError } from "./client";
import { fakeFetch, ids, testKey } from "../../test/fakeApi";

describe("ApiClient", () => {
  it("sends the key as a bearer token and encodes filters", async () => {
    const { fetch, calls } = fakeFetch({
      [`GET /v1/projects/${ids.project}/deliveries`]: () => ({ body: { deliveries: [], limit: 10, offset: 0, next_offset: null } }),
    });
    await new ApiClient(testKey, { fetch }).listDeliveries(ids.project, { status: "dead_letter", limit: 10, endpoint_id: undefined });
    expect(calls[0].auth).toBe(`Bearer ${testKey}`);
    expect(calls[0].search).toBe("?status=dead_letter&limit=10");
  });

  it("turns error envelopes into ApiError", async () => {
    const { fetch } = fakeFetch({
      [`POST /v1/deliveries/${ids.delivery}/replay`]: () => ({
        status: 409,
        body: { error: { code: "replay_in_progress", message: "an earlier replay of this delivery has not finished" } },
      }),
    });
    const err = await new ApiClient(testKey, { fetch }).replayDelivery(ids.delivery).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 409, code: "replay_in_progress" });
  });

  it("reports 401 so the app can sign out", async () => {
    const { fetch } = fakeFetch({ "GET /v1/projects": () => ({ status: 401, body: { error: { code: "unauthorized", message: "unauthorized" } } }) });
    const onUnauthorized = vi.fn();
    await expect(new ApiClient(testKey, { fetch, onUnauthorized }).listProjects()).rejects.toMatchObject({ status: 401 });
    expect(onUnauthorized).toHaveBeenCalledOnce();
  });

  it("maps network failures to a status 0 error", async () => {
    const failing = (() => Promise.reject(new TypeError("Failed to fetch"))) as typeof fetch;
    await expect(new ApiClient(testKey, { fetch: failing }).listProjects()).rejects.toMatchObject({ status: 0, code: "network_error" });
  });

  it("escapes path segments", async () => {
    const { fetch, calls } = fakeFetch({});
    await new ApiClient(testKey, { fetch }).getEvent("../projects").catch(() => undefined);
    expect(calls[0].path).toBe("/v1/events/..%2Fprojects");
  });
});
