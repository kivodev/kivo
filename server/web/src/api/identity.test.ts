import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/server";
import { client } from "./client";
import { identityHeaders } from "./identity";

const firefox = "Mozilla/5.0 (X11; Linux x86_64; rv:132.0) Gecko/20100101 Firefox/132.0";
const iphone =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1";

describe("identityHeaders", () => {
  it("is a header per field, leaving out what the browser can't say", () => {
    expect(identityHeaders(firefox)).toEqual({
      "Kivo-App": "kivo-web",
      "Kivo-App-Version": "dev",
      "Kivo-Platform": "browser",
      "Kivo-Device-Model": "Firefox 132",
    });
    expect(identityHeaders(iphone)).toMatchObject({ "Kivo-OS-Version": "17.5", "Kivo-Device-Model": "Safari 17" });
  });

  it("names the version the server wrote into the page", async () => {
    const meta = document.createElement("meta");
    meta.name = "kivo-version";
    meta.content = "0.3.1-dev+56c0018d";
    document.head.append(meta);
    vi.resetModules();
    try {
      const served = await import("./identity");
      expect(served.identityHeaders(firefox)["Kivo-App-Version"]).toBe("0.3.1-dev+56c0018d");
    } finally {
      meta.remove();
    }
  });

  it("goes with every API request", async () => {
    let header: string | null = null;
    server.use(
      http.get("*/api/v1/server", ({ request }) => {
        header = request.headers.get("Kivo-App");
        return HttpResponse.json({ qualityLadder: [] });
      }),
    );
    await client.GET("/api/v1/server");
    expect(header).toBe("kivo-web");
  });
});
