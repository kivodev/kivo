import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { problem, server } from "@/test/server";
import { client } from "./client";
import { errorCode, retryPageDelay, shouldRetry, shouldRetryPage } from "./problem";

/** What reading the server's info throws, as a query would see it. */
async function failure(answer: () => Response): Promise<unknown> {
  server.use(http.get("*/api/v1/server", answer));
  const { error } = await client.GET("/api/v1/server");
  expect(error).toBeDefined();
  return error;
}

const html = (status: number) =>
  new HttpResponse("<html>Bad gateway</html>", { status, headers: { "Content-Type": "text/html" } });

describe("no answer", () => {
  it("is a request that fails to send", () => {
    const error = new TypeError("Failed to fetch");
    expect(errorCode(error)).toBe("network");
    expect(shouldRetry(0, error)).toBe(true);
    expect(shouldRetry(2, error)).toBe(false);
  });

  it("is retried five times on a grid page, 1 s apart and doubling", () => {
    const error = new TypeError("Failed to fetch");
    expect(shouldRetryPage(4, error)).toBe(true);
    expect(shouldRetryPage(5, error)).toBe(false);
    expect([0, 1, 2, 3, 4].map((n) => retryPageDelay(n))).toEqual([1000, 2000, 4000, 8000, 16000]);
  });

  it.each([502, 503, 504])("is a gateway's %i without a problem document", async (status) => {
    const error = await failure(() => html(status));
    expect(errorCode(error)).toBe("network");
    expect(shouldRetry(0, error)).toBe(true);
  });

  it("is a gateway's 503 with no body", async () => {
    const error = await failure(() => new HttpResponse(null, { status: 503 }));
    expect(errorCode(error)).toBe("network");
  });
});

describe("the server's answer", () => {
  it.each(["transcodeBusy", "providerUnavailable", "notConfigured", "quotaUsed"] as const)(
    "is its own 503 %s, never retried",
    async (code) => {
      const error = await failure(() => problem(503, code));
      expect(errorCode(error)).toBe(code);
      expect(shouldRetry(0, error)).toBe(false);
    },
  );

  it.each([
    [404, "notFound"],
    [403, "forbidden"],
    [401, "unauthenticated"],
    [500, "internal"],
  ] as const)("is a %i %s, never retried", async (status, code) => {
    const error = await failure(() => problem(status, code));
    expect(errorCode(error)).toBe(code);
    expect(shouldRetry(0, error)).toBe(false);
  });

  it("is unknown when it isn't a problem document, and isn't retried", async () => {
    for (const status of [500, 404]) {
      const error = await failure(() => html(status));
      expect(errorCode(error)).toBe("unknown");
      expect(shouldRetry(0, error)).toBe(false);
    }
  });
});
