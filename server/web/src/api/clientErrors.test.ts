import { describe, expect, it, vi } from "vitest";
import { sent, serveMockApi } from "@/test/server";
import { client } from "./client";
import {
  type ClientErrorReport,
  createReporter,
  engineOf,
  isChunkError,
  reportClientError,
  reportOf,
  reportsPerMinute,
  routeTemplate,
  scrub,
  worthReporting,
} from "./clientErrors";

const report =
  (message: string): (() => ClientErrorReport) =>
  () => ({ kind: "error", message });

describe("createReporter", () => {
  it("sends only while signed in", () => {
    const send = vi.fn(() => Promise.resolve());
    let signedIn = false;
    const r = createReporter({ send, signedIn: () => signedIn });
    r(report("a"));
    expect(send).not.toHaveBeenCalled();
    signedIn = true;
    r(report("a"));
    expect(send).toHaveBeenCalledTimes(1);
  });

  it("sends a repeated message once", () => {
    const send = vi.fn(() => Promise.resolve());
    const r = createReporter({ send, signedIn: () => true });
    r(report("again"));
    r(report("again"));
    r(report("other"));
    expect(send.mock.calls.map((c: unknown[]) => (c[0] as ClientErrorReport).message)).toEqual(["again", "other"]);
  });

  it("sends a few a minute and drops the rest", () => {
    const send = vi.fn(() => Promise.resolve());
    let now = 0;
    const r = createReporter({ send, signedIn: () => true, now: () => now });
    for (let i = 0; i < 10; i++) r(report(`m${i}`));
    expect(send).toHaveBeenCalledTimes(reportsPerMinute);
    now = 60_000;
    r(report("later"));
    expect(send).toHaveBeenCalledTimes(reportsPerMinute + 1);
  });

  it("never throws or recurses", async () => {
    const r = createReporter({ send: () => Promise.reject(new Error("offline")), signedIn: () => true });
    expect(() =>
      r(() => {
        throw new Error("while building");
      }),
    ).not.toThrow();
    // A report that reports again while it's being built is dropped.
    const send = vi.fn(() => Promise.resolve());
    const nested = createReporter({ send, signedIn: () => true });
    nested(() => {
      nested(report("inner"));
      return { kind: "error", message: "outer" };
    });
    expect(send).toHaveBeenCalledTimes(1);
    expect(() => r(report("send fails"))).not.toThrow();
    await Promise.resolve();
  });
});

describe("reportOf", () => {
  it("keeps the top frames, without the message line, queries or typed text", () => {
    const error = new Error("no results for secret words");
    error.stack = [
      "Error: no results for secret words",
      ...Array.from({ length: 15 }, (_, i) => `    at f${i} (http://tv.local/tv/assets/index-abc.js?v=1:1:${i})`),
    ].join("\n");
    const r = reportOf("error", error, "/search", ["secret words"]);
    expect(r.message).toBe("Error: no results for …");
    expect(r.stack).toHaveLength(10);
    expect(r.stack?.[0]).toBe("at f0 (http://tv.local/tv/assets/index-abc.js)");
    expect(r.route).toBe("/search");
  });

  it("bounds the message and each frame", () => {
    const error = new Error("x".repeat(900));
    error.stack = `Error: ${"x".repeat(900)}\n    at ${"y".repeat(900)}`;
    const r = reportOf("render", error, "/", []);
    expect(r.message).toHaveLength(500);
    expect(r.stack?.[0]).toHaveLength(300);
  });

  it("names what isn't an Error without its contents", () => {
    expect(reportOf("unhandledRejection", { password: "hunter2" }, "/", []).message).toBe(
      "a non-Error value was thrown",
    );
  });
});

describe("scrub", () => {
  it("cuts every URL's query and fragment", () => {
    expect(scrub("GET http://nas:8484/api/v1/search?q=private#x failed", [])).toBe(
      "GET http://nas:8484/api/v1/search failed",
    );
  });

  it("hides the page's query values and the focused field's text", () => {
    window.history.replaceState(null, "", "/search?q=my+holiday");
    const input = document.createElement("input");
    input.value = "typed here";
    document.body.append(input);
    input.focus();
    expect(scrub("no match for my holiday or typed here")).toBe("no match for … or …");
    input.remove();
    window.history.replaceState(null, "", "/");
  });
});

describe("routeTemplate", () => {
  it("names IDs by a parameter and drops the base path", () => {
    expect(routeTemplate("/kivo/works/0198a3c2-7e41-7c3b-9b1a-5f1c2d3e4f50/files", "/kivo/")).toBe("/works/$id/files");
    expect(routeTemplate("/people/Jane%20Doe", "/")).toBe("/people/$id");
    expect(routeTemplate("/", "/")).toBe("/");
  });
});

describe("engineOf", () => {
  it("reads a Samsung TV's Tizen and Chromium versions", () => {
    const ua =
      "Mozilla/5.0 (SMART-TV; LINUX; Tizen 9.0) AppleWebKit/537.36 (KHTML, like Gecko) 120.0.6099.5/9.0 Chrome/120.0.6099.5 TV Safari/537.36";
    expect(engineOf(ua)).toEqual({ platform: "tizen", tizenVersion: "9.0", chromiumVersion: "120" });
  });

  it("names webOS and other browsers", () => {
    expect(engineOf("Mozilla/5.0 (Web0S; Linux/SmartTV) Chrome/108.0.5359.211").platform).toBe("webos");
    expect(engineOf("Mozilla/5.0 (Macintosh) Version/17.0 Safari/605.1.15")).toEqual({ platform: "browser" });
  });
});

describe("which errors are reported", () => {
  it("recognises a chunk that failed to load", () => {
    expect(isChunkError(new TypeError("Failed to fetch dynamically imported module: http://x/assets/a.js"))).toBe(true);
    expect(isChunkError(new Error("boom"))).toBe(false);
    expect(worthReporting(new TypeError("Failed to fetch dynamically imported module: http://x/assets/a.js"))).toBe(
      true,
    );
  });

  it("leaves out the server's problems and requests that got no answer", () => {
    expect(worthReporting({ type: "about:blank", title: "", status: 500, code: "internal", requestId: "" })).toBe(
      false,
    );
    expect(worthReporting(new TypeError("Failed to fetch"))).toBe(false);
    expect(worthReporting(new DOMException("timed out", "TimeoutError"))).toBe(false);
    expect(worthReporting(new TypeError("Cannot read properties of undefined (reading 'id')"))).toBe(true);
  });
});

it("sends nothing from the web build, which has the browser's console", async () => {
  serveMockApi();
  await client.GET("/api/v1/users/me");
  reportClientError("error", new Error("boom"));
  await new Promise((resolve) => setTimeout(resolve, 20));
  expect(sent("POST", "/client-errors")).toHaveLength(0);
});
