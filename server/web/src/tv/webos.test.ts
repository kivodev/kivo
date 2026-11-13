import { afterEach, expect, it, vi } from "vitest";
import { logWebosHdr, webosDeviceHdr, webosHdrPanel } from "./webos";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function display(hdr: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({ matches: hdr && query === "(dynamic-range: high)" }));
}

it("says whether an LG TV's panel shows HDR", () => {
  vi.stubGlobal("PalmSystem", {});
  display(true);
  expect(webosHdrPanel()).toBe(true);
  display(false);
  expect(webosHdrPanel()).toBe(false);
});

it("can't say off an LG TV", () => {
  display(true);
  expect(webosHdrPanel()).toBeUndefined();
});

it("can't say where the display can't be asked", () => {
  vi.stubGlobal("PalmSystem", {});
  vi.stubGlobal("matchMedia", () => {
    throw new Error("unsupported");
  });
  expect(webosHdrPanel()).toBeUndefined();
});

/** An LG TV's service bridge, which answers getConfigs with the response given, and the calls it was asked. */
function bridge(response: string) {
  const calls: [string, unknown][] = [];
  vi.stubGlobal(
    "PalmServiceBridge",
    class {
      onservicecallback = (_: string) => {};
      call(uri: string, parameters: string) {
        calls.push([uri, JSON.parse(parameters)]);
        this.onservicecallback(response);
      }
    },
  );
  return calls;
}

it("asks LG's device info whether the TV shows HDR10 and Dolby Vision, as webOS.deviceInfo does", async () => {
  vi.stubGlobal("PalmSystem", {});
  const calls = bridge(
    JSON.stringify({
      returnValue: true,
      configs: { "tv.model.supportHDR": true, "tv.config.supportDolbyHDRContents": false },
    }),
  );
  expect(await webosDeviceHdr()).toEqual({ hdr10: true, dolbyVision: false });
  expect(calls).toEqual([
    [
      "luna://com.webos.service.config/getConfigs",
      { configNames: ["tv.model.supportHDR", "tv.config.supportDolbyHDRContents"] },
    ],
  ]);
});

it("can't say what LG's device info doesn't answer", async () => {
  vi.stubGlobal("PalmSystem", {});
  bridge(JSON.stringify({ returnValue: false, errorText: "denied" }));
  expect(await webosDeviceHdr()).toBeUndefined();
  bridge("not json");
  expect(await webosDeviceHdr()).toBeUndefined();
});

it("can't ask LG's device info off an LG TV", async () => {
  const calls = bridge("{}");
  expect(await webosDeviceHdr()).toBeUndefined();
  expect(calls).toEqual([]);
});

it("logs both HDR answers on an LG TV, for checking them against each other on a real panel", async () => {
  const info = vi.spyOn(console, "info").mockImplementation(() => {});
  await logWebosHdr();
  expect(info).not.toHaveBeenCalled();

  vi.stubGlobal("PalmSystem", {});
  display(true);
  bridge(JSON.stringify({ configs: { "tv.model.supportHDR": true, "tv.config.supportDolbyHDRContents": true } }));
  await logWebosHdr();
  expect(info).toHaveBeenCalledWith(
    "Kivo HDR check: display (dynamic-range: high) true; LG device info hdr10 true, dolbyVision true",
  );

  bridge(JSON.stringify({ returnValue: false }));
  display(false);
  await logWebosHdr();
  expect(info).toHaveBeenLastCalledWith("Kivo HDR check: display (dynamic-range: high) false; LG device info unknown");
});
