// What the LG TV app (clients/webos) asks of webOS. Its page has the
// PalmSystem global, which a browser, a Samsung TV and tests don't, and
// everything here does nothing without it. Leaving the app needs nothing of
// webOS: closing the window ends it (exit.ts). The remote's Back and media
// keys arrive unasked (features/player/keys.ts), and webOS keeps its
// screensaver off while a video element plays.

type PalmServiceBridge = { onservicecallback: (response: string) => void; call(uri: string, parameters: string): void };
type WebosGlobals = { PalmSystem?: object; PalmServiceBridge?: new () => PalmServiceBridge };

/**
 * Whether the TV's panel shows HDR, by the display's dynamic range, which
 * the capability report declares HDR by. Undefined off an LG TV, or where
 * the display can't be asked.
 */
export function webosHdrPanel(): boolean | undefined {
  if (!(window as unknown as WebosGlobals).PalmSystem) return undefined;
  try {
    return matchMedia("(dynamic-range: high)").matches;
  } catch {
    return undefined;
  }
}

/** What LG's own device info says the TV shows. */
export type WebosDeviceHdr = { hdr10: boolean; dolbyVision: boolean };

/**
 * What LG's own device info says of HDR10 and Dolby Vision: the settings
 * webOS.deviceInfo (LG's webOSTV.js) reads for its hdr10 and dolbyVision,
 * asked of the TV directly, without the library. Undefined off an LG TV, or
 * where the TV doesn't answer them. Only for checking against
 * webosHdrPanel on a real panel; the capability report doesn't use it.
 */
export function webosDeviceHdr(): Promise<WebosDeviceHdr | undefined> {
  const Bridge = (window as unknown as WebosGlobals).PalmServiceBridge;
  if (!(window as unknown as WebosGlobals).PalmSystem || !Bridge) return Promise.resolve(undefined);
  return new Promise((resolve) => {
    try {
      const bridge = new Bridge();
      bridge.onservicecallback = (response) => {
        try {
          const configs = (JSON.parse(response) as { configs?: Record<string, unknown> }).configs;
          resolve(
            configs
              ? {
                  hdr10: configs["tv.model.supportHDR"] === true,
                  dolbyVision: configs["tv.config.supportDolbyHDRContents"] === true,
                }
              : undefined,
          );
        } catch {
          resolve(undefined);
        }
      };
      bridge.call(
        "luna://com.webos.service.config/getConfigs",
        JSON.stringify({ configNames: ["tv.model.supportHDR", "tv.config.supportDolbyHDRContents"] }),
      );
    } catch {
      resolve(undefined);
    }
  });
}

/**
 * Logs, once on an LG TV, both answers to whether its panel shows HDR: the
 * display's, which the capability report goes by, and LG's device info. A
 * tester reads the line through ares-inspect, on an HDR LG and a non-HDR
 * one, to see whether the two agree before the report relies on the first.
 */
export async function logWebosHdr(): Promise<void> {
  if (!(window as unknown as WebosGlobals).PalmSystem) return;
  const device = await webosDeviceHdr();
  console.info(
    `Kivo HDR check: display (dynamic-range: high) ${webosHdrPanel() ?? "unknown"}; LG device info ${
      device ? `hdr10 ${device.hdr10}, dolbyVision ${device.dolbyVision}` : "unknown"
    }`,
  );
}
