// What the user agent says about this browser, read in one place for the
// whole app: the capability report's rules, a session's name, the identity
// headers, a client error's engine, and whether this is a TV's or a phone's
// browser.

/** The browser, as the app tells browsers apart. */
export type Browser = {
  safari: boolean;
  firefox: boolean;
  chrome: boolean;
  edge: boolean;
  ios: boolean;
  mac: boolean;
  android: boolean;
  mobile: boolean;
  /** A TV's browser, which has a remote rather than a keyboard. */
  tv: boolean;
  /** The Samsung TV's Tizen version, such as 6.5; 0 elsewhere. */
  tizen: number;
  /** An LG TV. */
  webos: boolean;
  /** The LG TV's webOS version, such as 5 or 22; 0 elsewhere, or where its user agent doesn't tell. */
  webosVersion: number;
  /** The browser's major version; for Safari, its Version/. */
  version: number;
  /** The engine's Chromium version, such as "120.0.6099.5", where the user agent names it. */
  chromium?: string;
};

/**
 * The browser from its user agent and vendor. Chrome on iOS is Safari
 * underneath, and an iPad reports a Mac with a touch screen. A Samsung TV
 * names its engine without "Chrome/", as a bare version after
 * "like Gecko) " with the Tizen version after a slash: "120.0.6099.5/9.0".
 */
export function browserOf(userAgent: string, vendor: string, touchPoints = 0): Browser {
  const ua = userAgent.toLowerCase();
  // A Samsung TV's browser names Chrome and Safari too, but is neither.
  const tizenMatch = /\btizen ?(\d+)(?:\.(\d+))?/.exec(ua);
  const tizen = tizenMatch ? Number(tizenMatch[1]) + Number(tizenMatch[2] ?? 0) / 10 : 0;
  const edge = /\bedg(a|ios)?\//.test(ua);
  const safari = !tizen && vendor.startsWith("Apple");
  const firefox = !safari && ua.includes("firefox/");
  const chrome = !tizen && !safari && !firefox && !edge && ua.includes("chrome/");
  const ipadAsMac = ua.includes("mac os x") && touchPoints > 1;
  const ios = /iphone|ipad|ipod/.test(ua) || ipadAsMac;
  const android = ua.includes("android");
  const token = safari
    ? /version\/(\d+)/
    : edge
      ? /edg(?:a|ios)?\/(\d+)/
      : firefox
        ? /firefox\/(\d+)/
        : /chrome\/(\d+)/;
  const webos = /web0s|webos/.test(ua);
  const chromium = (/\bChrome\/(\d+(?:\.\d+)*)/.exec(userAgent) ??
    /like Gecko\) (\d+(?:\.\d+){3})\//.exec(userAgent))?.[1];
  return {
    safari,
    firefox,
    chrome,
    edge,
    ios,
    mac: ua.includes("mac os x") && !ios,
    android,
    mobile: ios || android,
    tv: /tizen|web0s|webos|smart-tv|smarttv|\baft[a-z]|crkey|bravia/.test(ua),
    tizen,
    webos,
    webosVersion: webos ? webosVersion(ua) : 0,
    version: Number(token.exec(ua)?.[1] ?? 0),
    ...(chromium ? { chromium } : {}),
  };
}

/**
 * An LG TV's webOS version, which its user agent doesn't name, from the
 * Chrome version each webOS release ships, by Jellyfin's web client's map
 * (src/scripts/browser.js). webOS 1 and 2 name no Chrome, only Safari. LG's
 * own browser (NetCast) carries a Chrome of its own, so it is 0, as is
 * anything else the map doesn't place.
 */
function webosVersion(ua: string): number {
  const chrome = Number(/chrome\/(\d+)/.exec(ua)?.[1] ?? 0);
  if (!chrome) {
    const safari = Number(/safari\/(\d+)/.exec(ua)?.[1] ?? 0);
    return safari >= 538 ? 2 : safari >= 537 ? 1 : 0;
  }
  if (ua.includes("netcast")) return 0;
  const releases: [number, number][] = [
    [120, 25],
    [108, 24],
    [94, 23],
    [87, 22],
    [79, 6],
    [68, 5],
    [53, 4],
    [38, 3],
    [34, 2],
    [26, 1],
  ];
  return releases.find(([from]) => chrome >= from)?.[1] ?? 0;
}

/** This browser. */
export function currentBrowser(): Browser {
  return browserOf(navigator.userAgent, navigator.vendor, navigator.maxTouchPoints);
}

/**
 * This browser's name, such as "Firefox on Linux x86_64". It is used as
 * the session's name, which Where you're signed in and the admin's Now
 * playing show.
 */
export function browserName(): string {
  const b = currentBrowser();
  const browser = b.safari ? "Safari" : b.firefox ? "Firefox" : b.chrome || b.edge ? "Chrome" : "Web browser";
  const platform =
    (navigator as { userAgentData?: { platform: string } }).userAgentData?.platform || navigator.platform;
  return (platform ? `${browser} on ${platform}` : browser).slice(0, 100);
}

/** The browser's family and major version, such as "Firefox 132", from its user agent. */
export function deviceModel(userAgent: string): string | undefined {
  const families: [string, RegExp][] = [
    ["Edge", /Edg(?:e|A|iOS)?\/(\d+)/],
    ["Firefox", /(?:Firefox|FxiOS)\/(\d+)/],
    ["Chrome", /(?:Chrome|CriOS)\/(\d+)/],
    ["Safari", /Version\/(\d+)[\d.]* (?:Mobile\/\S+ )?Safari\//],
  ];
  for (const [family, pattern] of families) {
    const major = pattern.exec(userAgent)?.[1];
    if (major) return `${family} ${major}`;
  }
  return undefined;
}

/**
 * The operating system's version, where the user agent tells it truly:
 * iOS and iPadOS ("17.5") and Android ("14"). Desktop browsers freeze
 * theirs, so it is left out there.
 */
export function osVersion(userAgent: string): string | undefined {
  const ios = /(?:iPhone|iPad|iPod)[^)]*? OS (\d+(?:_\d+)*)/.exec(userAgent)?.[1];
  if (ios) return ios.replaceAll("_", ".");
  return /Android (\d+(?:\.\d+)*)/.exec(userAgent)?.[1];
}
