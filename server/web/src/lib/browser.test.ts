import { describe, expect, it } from "vitest";
import { browserOf, deviceModel, osVersion } from "./browser";

const chrome =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36";
const edge = `${chrome} Edg/141.0.3537.71`;
const android =
  "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Mobile Safari/537.36";
const firefox = "Mozilla/5.0 (X11; Linux x86_64; rv:132.0) Gecko/20100101 Firefox/132.0";
const firefoxMac = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:143.0) Gecko/20100101 Firefox/143.0";
const mac =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Safari/605.1.15";
const iphone =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1";
const chromeOnIphone =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 18_1_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/141.0.7390.41 Mobile/15E148 Safari/604.1";
// The 2025 Frame's own user agent, which names its engine without "Chrome/".
const frame2025 =
  "Mozilla/5.0 (SMART-TV; LINUX; Tizen 9.0) AppleWebKit/537.36 (KHTML, like Gecko) 120.0.6099.5/9.0 TV Safari/537.36";
const tizen65 =
  "Mozilla/5.0 (SMART-TV; LINUX; Tizen 6.5) AppleWebKit/537.36 (KHTML, like Gecko) 85.0.4183.93/6.5 TV Safari/537.36";
const webOs =
  "Mozilla/5.0 (Web0S; Linux/SmartTV) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.5359.211 Safari/537.36 WebAppManager";

const google = "Google Inc.";
const apple = "Apple Computer, Inc.";

describe("browserOf", () => {
  it("tells browsers apart as the capability report's rules do", () => {
    expect(browserOf(chrome, google)).toMatchObject({ chrome: true, edge: false, mobile: false, version: 141 });
    expect(browserOf(edge, google)).toMatchObject({ chrome: false, edge: true, version: 141 });
    expect(browserOf(android, google)).toMatchObject({ chrome: true, android: true, mobile: true, tv: false });
    expect(browserOf(firefoxMac, "")).toMatchObject({ firefox: true, mac: true, version: 143 });
    expect(browserOf(mac, apple)).toMatchObject({ safari: true, mac: true, ios: false, version: 26 });
    expect(browserOf(iphone, apple)).toMatchObject({ safari: true, ios: true, mac: false, mobile: true });
    // Chrome on iOS is WebKit underneath, and an iPad reports a Mac with a touch screen.
    expect(browserOf(chromeOnIphone, apple).safari).toBe(true);
    expect(browserOf(mac, apple, 5)).toMatchObject({ ios: true, mac: false });
  });

  it("reads a Samsung TV, which names Chrome and Safari but is neither, and its engine without Chrome/", () => {
    expect(browserOf(tizen65, "")).toMatchObject({ tizen: 6.5, chrome: false, safari: false, tv: true });
    expect(browserOf(`${chrome} Tizen 5.0`, apple)).toMatchObject({ tizen: 5, chrome: false, safari: false });
    expect(browserOf(frame2025, "")).toMatchObject({ tizen: 9, chromium: "120.0.6099.5" });
  });

  it("reads an LG TV, which gives no webOS version", () => {
    expect(browserOf(webOs, "")).toMatchObject({ webos: true, tizen: 0, tv: true, chromium: "108.0.5359.211" });
  });

  it("says nothing of an engine it doesn't name", () => {
    expect(browserOf(mac, apple).chromium).toBeUndefined();
  });
});

describe("deviceModel", () => {
  it("names the browser's family and major version", () => {
    expect(deviceModel(firefox)).toBe("Firefox 132");
    expect(deviceModel(chrome)).toBe("Chrome 141");
    expect(deviceModel(edge)).toBe("Edge 141");
    expect(deviceModel(iphone)).toBe("Safari 17");
    expect(deviceModel(chromeOnIphone)).toBe("Chrome 141");
    expect(deviceModel(mac)).toBe("Safari 26");
    expect(deviceModel("curl/8.0")).toBeUndefined();
  });
});

describe("osVersion", () => {
  it("reads iOS and Android, and leaves out the versions desktop browsers freeze", () => {
    expect(osVersion(iphone)).toBe("17.5");
    expect(osVersion(chromeOnIphone)).toBe("18.1.1");
    expect(osVersion(android)).toBe("14");
    expect(osVersion(mac)).toBeUndefined();
    expect(osVersion(chrome)).toBeUndefined();
  });
});
