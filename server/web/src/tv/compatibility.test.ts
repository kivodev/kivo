import { describe, expect, it, vi } from "vitest";
import { compareVersions, compatibility, minServerVersion } from "./compatibility";

const app = vi.hoisted(() => ({ version: "0.5.0" }));
vi.mock("./versions", () => ({
  get appVersion() {
    return app.version;
  },
}));

describe("comparing versions", () => {
  it("goes by major, minor and patch", () => {
    expect(compareVersions("0.5.0", "0.4.0")).toBeGreaterThan(0);
    expect(compareVersions("0.4.1", "0.4.0")).toBeGreaterThan(0);
    expect(compareVersions("1.0.0", "0.10.9")).toBeGreaterThan(0);
    expect(compareVersions("0.10.0", "0.9.0")).toBeGreaterThan(0);
    expect(compareVersions("0.4.0", "0.5.0")).toBeLessThan(0);
    expect(compareVersions("0.4.0", "0.4.0")).toBe(0);
    expect(compareVersions("v0.4.0", "0.4.0")).toBe(0);
  });

  it("takes a build past a release as that release", () => {
    expect(compareVersions("0.3.0-dev+abc1234", "0.3.0")).toBe(0);
    expect(compareVersions("0.3.0+abc1234", "0.3.0")).toBe(0);
    expect(compareVersions("v0.3.0-3-gabc1234-dirty", "0.3.0")).toBe(0);
    expect(compareVersions("0.3.0+78", "0.3.0")).toBe(0);
    expect(compareVersions("0.3.1-dev+56c0018d", "0.3.0")).toBeGreaterThan(0);
    expect(compareVersions("0.3.0-dev+abc1234", "0.4.0")).toBeLessThan(0);
  });

  it("compares a version that isn't a release with nothing", () => {
    for (const odd of [
      "dev",
      "d76b7e0",
      "",
      " ",
      "latest",
      "0.5",
      "1.0",
      "0.3.x",
      "0.3.0.1",
      "0..3",
      "-1.0.0",
      "0.3.0 (12)",
    ]) {
      expect(compareVersions(odd, "0.3.0"), odd).toBeUndefined();
      expect(compareVersions("0.3.0", odd), odd).toBeUndefined();
    }
  });
});

describe("the app and a server", () => {
  it("say the server is too old when it's older than the app works with", () => {
    expect(compatibility("0.1.0-dev+abc1234", "0.3.0")).toEqual({
      state: "serverTooOld",
      serverVersion: "0.1.0",
      minServerVersion,
      appVersion: "0.5.0",
    });
  });

  it("say the app is too old when it's older than the server supports", () => {
    expect(compatibility(minServerVersion, "0.6.0")).toEqual({
      state: "appTooOld",
      appVersion: "0.5.0",
      minAppVersion: "0.6.0",
    });
  });

  it("work together at each other's oldest, and a build past a release is not older than it", () => {
    expect(compatibility(minServerVersion, "0.5.0")).toEqual({ state: "compatible" });
    expect(compatibility(`${minServerVersion}-dev+abc1234`, "0.5.0")).toEqual({ state: "compatible" });
  });

  it("are never kept apart by a missing or odd version", () => {
    expect(compatibility(minServerVersion, undefined)).toEqual({ state: "compatible" });
    for (const odd of ["dev", "d76b7e0", ""]) {
      expect(compatibility(odd, "0.1.0"), odd).toEqual({ state: "compatible" });
      expect(compatibility(minServerVersion, odd), odd).toEqual({ state: "compatible" });
      app.version = odd;
      expect(compatibility(minServerVersion, "9.0.0"), odd).toEqual({ state: "compatible" });
      app.version = "0.5.0";
    }
  });
});
