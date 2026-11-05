import { beforeEach, describe, expect, it, vi } from "vitest";
import { testServer } from "@/test/packagedSetup";
import { packagedClientId, packagedServer, packagedToken, setPackagedServer, setPackagedToken } from "./packaged";
import { restartApp } from "./restart";

const kept = { address: "https://kivo.example", serverId: "s1" };

/** The module as a newly started app has it, reading whatever storage holds now. */
async function started() {
  vi.resetModules();
  return import("./packaged");
}

/** Storage as a run that kept `kept` left it. */
function keptBefore() {
  localStorage.setItem("kivo.server", JSON.stringify(kept));
}

describe("packaged app storage", () => {
  beforeEach(() => localStorage.clear());

  it("keeps the server without a trailing slash", () => {
    setPackagedServer({ address: "https://kivo.example/", serverId: "s1" });
    expect(packagedServer()).toEqual(kept);
  });

  it("forgets the token with the server", () => {
    setPackagedToken("t");
    setPackagedServer(undefined);
    expect(packagedServer()).toBeUndefined();
    expect(packagedToken()).toBeUndefined();
  });

  it("starts the app again on another server, forgetting the token, but not on the server it started with", () => {
    setPackagedToken("t");
    expect(setPackagedServer({ ...testServer, address: `${testServer.address}/` })).toBe(false);
    expect(packagedToken()).toBe("t");
    expect(restartApp).not.toHaveBeenCalled();

    expect(setPackagedServer(kept)).toBe(true);
    expect(restartApp).toHaveBeenCalledOnce();
    expect(packagedToken()).toBeUndefined();
    expect(JSON.parse(localStorage.getItem("kivo.server") ?? "null")).toEqual(kept);
  });

  it("goes on without starting again when the server it started with is chosen again after being forgotten", () => {
    setPackagedServer(undefined);
    expect(setPackagedServer(testServer)).toBe(false);
    expect(restartApp).not.toHaveBeenCalled();
  });

  it("keeps no token for a server other than the one it started with", () => {
    setPackagedServer(undefined);
    setPackagedToken("late");
    expect(packagedToken()).toBeUndefined();
    setPackagedServer(kept);
    setPackagedToken("late");
    expect(packagedToken()).toBeUndefined();
  });

  it("ignores a kept server that isn't one", async () => {
    localStorage.setItem("kivo.server", JSON.stringify({ address: 1 }));
    const app = await started();
    expect(app.packagedServer()).toBeUndefined();
  });

  it("makes the client ID once", () => {
    const first = packagedClientId();
    expect(packagedClientId()).toBe(first);
  });

  it("starts with what the last run kept", async () => {
    keptBefore();
    const last = await started();
    last.setPackagedToken("t");
    const id = last.packagedClientId();
    const app = await started();
    expect(app.packagedServer()).toEqual(kept);
    expect(app.packagedToken()).toBe("t");
    expect(app.packagedClientId()).toBe(id);
  });

  it("reads storage once, as it starts, not on every call", async () => {
    keptBefore();
    localStorage.setItem("kivo.token", "t");
    const app = await started();
    app.packagedServer();
    app.packagedClientId();
    const read = vi.spyOn(Storage.prototype, "getItem");
    for (let i = 0; i < 3; i++) {
      app.packagedServer();
      app.packagedToken();
      app.packagedClientId();
    }
    expect(read).not.toHaveBeenCalled();
  });

  it("keeps working for this run when storage refuses to keep anything", async () => {
    keptBefore();
    const app = await started();
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("full", "QuotaExceededError");
    });
    app.setPackagedToken("t");
    expect(app.packagedServer()).toEqual(kept);
    expect(app.packagedToken()).toBe("t");
    expect(app.packagedClientId()).toBe(app.packagedClientId());
  });
});
