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
  /** An LG TV, which doesn't give its webOS version. */
  webos: boolean;
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
    webos: /web0s|webos/.test(ua),
    version: Number(token.exec(ua)?.[1] ?? 0),
    ...(chromium ? { chromium } : {}),
  };
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
