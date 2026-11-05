import { readdirSync, readFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { lingui } from "@lingui/vite-plugin";
import babel from "@rolldown/plugin-babel";
import tailwindcss from "@tailwindcss/vite";
import react, { reactCompilerPreset } from "@vitejs/plugin-react";
import license, { type Dependency } from "rollup-plugin-license";
import type { Plugin } from "vite";
import { defineConfig } from "vitest/config";

// The dev server proxies the API to a running Kivo server (KIVO_URL), or
// `npm run dev:mock` serves the MSW mocks instead.
const kivoUrl = process.env.KIVO_URL ?? "http://localhost:8484";

// The design tokens and display tables the native apps share.
const shared = fileURLToPath(new URL("../../shared", import.meta.url));

// The TV build is the same client with VITE_KIVO_TV=1. It's built only as
// the packaged TV app (below); the server serves the web build alone. The
// dev server runs it unpackaged too, for the TV's browser tests.
const tv = process.env.VITE_KIVO_TV === "1";

// The packaged TV app is the TV build with VITE_KIVO_PACKAGED=1
// as well, written to dist-packaged for clients/tizen/package.sh. It sits
// beside dist rather than in it, because the server embeds all of dist.
const packaged = process.env.VITE_KIVO_PACKAGED === "1";

// The packaged app's version is its package's, from clients/tizen/config.xml,
// which Settings › About shows beside the server's (src/tv/versions.ts).
function packageVersion(): string {
  const config = readFileSync(new URL("../../clients/tizen/config.xml", import.meta.url), "utf8");
  const version = /<widget[^>]*\sversion="([^"]+)"/.exec(config)?.[1];
  if (version === undefined) throw new Error("clients/tizen/config.xml has no widget version");
  return version;
}

// The client's build, which its requests and error reports name
// (src/api/identity.ts): the time it was built, to the second, as the
// client has no version of its own apart from the server it's embedded in.
const buildTime = new Date().toISOString().replace(/\.\d+Z$/, "Z");

// The TV build's splash shows from the moment the page loads, before its
// code has run: a TV takes seconds to load and start it, and a blank screen
// looks broken. It goes in #root, which React replaces on its first render
// with the same splash (src/tv/Splash.tsx). The packaged page's CSP allows
// only its own stylesheets, so its styles are a file (src/tv/splash.css),
// which the build folds into the TV build's stylesheet, as it does the TV
// build's font (src/tv/fonts.css). The web build's index.html and
// stylesheet stay as they are.
//
// The root carries data-tv from the start, which src/tv/start.ts would set
// only once the app's code runs: it switches the tokens to the TV's scale,
// rem with them, so without it the splash's icon showed at the web's size
// and then grew as the code arrived. The dev server's app stylesheet comes
// with the code, so there it's linked as well, for the splash to have its
// tokens from the first frame as in the build.
function tvSplash(): Plugin {
  return {
    name: "kivo-tv-splash",
    transformIndexHtml: {
      order: "pre",
      handler: (html, { server }) =>
        replaceOnce(html, /<html\b/, '<html data-tv=""', "kivo-tv-splash")
          .replace(
            '<div id="root"></div>',
            '<div id="root"><div data-part="tv-splash" role="status" aria-label="Kivo is starting">' +
              '<img src="/icon.svg" alt="" /><span data-part="tv-splash-spinner"></span></div></div>',
          )
          .replace(
            "</head>",
            (server ? '  <link rel="stylesheet" href="/src/theme/index.css" />\n  ' : "") +
              '  <link rel="stylesheet" href="/src/tv/fonts.css" />\n' +
              '    <link rel="stylesheet" href="/src/tv/splash.css" />\n  </head>',
          ),
    },
  };
}

// The packaged app's policy, the server's with the server allowed where it
// was 'self'. A file: page has no server to send a header, so
// it's a meta element. Images keep 'self' as well, for the package's own
// icon and logo, which the splash and the first screens show. There is no
// frame-ancestors: Chromium ignores it in a meta element and logs an error
// for it.
const packagedPolicy = [
  "default-src 'self'",
  "script-src 'self' 'wasm-unsafe-eval'",
  "worker-src 'self' blob:",
  "connect-src https: http: wss: ws:",
  "img-src 'self' https: http: data: blob:",
  "media-src https: http: blob: data:",
  "style-src 'self'",
  // The <object type="application/avplayer"> that Samsung's native player
  // draws through (features/player/avplay.ts). It loads nothing.
  "object-src 'self'",
  "base-uri 'none'",
].join("; ");

/** html with from replaced by to, failing the build when from isn't there. */
function replaceOnce(html: string, from: string | RegExp, to: string, plugin = "kivo-packaged-page"): string {
  const replaced = html.replace(from, to);
  if (replaced === html) throw new Error(`${plugin}: index.html has no ${from}`);
  return replaced;
}

// The packaged app's page is file:///index.html in the package. It has no
// <base href>, which would point at the file system's root, so its relative
// asset URLs ("./", as base below) resolve beside it. It carries the policy,
// and no web app manifest or home-screen icons, which only an installed web
// page uses. Samsung's
// webapis (the panel's resolution, src/tv/tizen.ts) exist only on a page that
// loads its script, which the TV resolves to a file of its own; the tag is
// added after the build so the bundler leaves its path alone, and ahead of
// the app's script so webapis is there when the app starts.
function packagedPage(): Plugin {
  return {
    name: "kivo-packaged-page",
    apply: "build",
    transformIndexHtml: {
      order: "pre",
      handler: (html) => {
        let page = replaceOnce(html, /\n\s*<!-- The server rewrites this[^>]*-->\n\s*<base href="\/" \/>/, "");
        page = replaceOnce(page, /\n\s*<link rel="manifest" href="\/manifest.json" \/>/, "");
        page = replaceOnce(page, /\n\s*<link rel="apple-touch-icon" href="\/apple-touch-icon.png" \/>/, "");
        page = replaceOnce(
          page,
          '<meta charset="UTF-8" />',
          `<meta charset="UTF-8" />\n    <meta http-equiv="Content-Security-Policy" content="${packagedPolicy}" />`,
        );
        return {
          html: page,
          tags: [{ tag: "script", attrs: { src: "$WEBAPIS/webapis/webapis.js" }, injectTo: "head" }],
        };
      },
    },
    // The manifest and the icons only it and a phone's home screen use come
    // from public/ with the rest, and are left out.
    writeBundle({ dir }) {
      for (const file of ["manifest.json", "apple-touch-icon.png", "icon-192.png", "icon-512.png"]) {
        rmSync(join(dir ?? "", file), { force: true });
      }
    },
  };
}

// A TV build that isn't the packaged app has nowhere to go: the server
// serves only the web build.
function packagedOnly(): Plugin {
  return {
    name: "kivo-tv-packaged-only",
    apply: "build",
    buildStart() {
      throw new Error("The TV build is built only as the packaged app: npm run build:packaged");
    },
  };
}

// `npm run dev:mock` serves MSW's service worker and the playable scenario's
// media from mock-public, beside the public files, which the screens' logos
// and the splash's icon are among. Neither is ever in a build.
function mockWorker(): Plugin {
  const worker = fileURLToPath(new URL("./mock-public/mockServiceWorker.js", import.meta.url));
  const media = fileURLToPath(new URL("./mock-public/mock-media/", import.meta.url));
  return {
    name: "kivo-mock-worker",
    apply: "serve",
    configureServer(server) {
      server.middlewares.use("/mockServiceWorker.js", (_req, res) => {
        res.setHeader("Content-Type", "text/javascript");
        res.end(readFileSync(worker));
      });
      server.middlewares.use("/mock-media/", (req, res, next) => {
        const name = (req.url ?? "").split("?")[0]?.replace(/^\//, "") ?? "";
        if (!/^[\w.-]+$/.test(name)) return next();
        res.setHeader("Content-Type", "video/mp4");
        res.end(readFileSync(`${media}${name}`));
      });
    },
  };
}

type Credit = { name: string; version?: string; license: string; url?: string; text?: string };

/** The package in node_modules that file is in, if it's in one. */
function packageOf(file: string): string | undefined {
  const parts = file.split(/(?:^|[\\/])node_modules[\\/]/);
  const rest = parts.length > 1 ? parts[parts.length - 1]?.split(/[\\/]/) : undefined;
  if (!rest?.[0]) return undefined;
  return rest[0].startsWith("@") ? `${rest[0]}/${rest[1]}` : rest[0];
}

/** A package's license as its package.json and license file give it, for one only a stylesheet brings in. */
function stylesheetPackage(name: string): Credit {
  const dir = fileURLToPath(new URL(`./node_modules/${name}/`, import.meta.url));
  const pkg = JSON.parse(readFileSync(join(dir, "package.json"), "utf8")) as { version?: string; license?: string };
  if (!pkg.license) throw new Error(`kivo-licenses: ${name} has no license`);
  const file = readdirSync(dir).find((f) => /^(licen[cs]e|copying)/i.test(f));
  return {
    name,
    version: pkg.version,
    license: pkg.license,
    text: file ? readFileSync(join(dir, file), "utf8") : undefined,
  };
}

/**
 * The open source licenses Settings › About › Credits lists
 * (src/features/user/licenses.ts): each package in the bundle, the ones its
 * code imports as rollup-plugin-license finds them and the ones only a
 * stylesheet brings in, such as the TV's Inter, from their assets. Then the
 * player's fonts (their OFL.txt) and the terms the bundle can't report
 * (shared/credits/terms.json). It's licenses.js beside index.html, a module
 * the page imports only when the list opens, so the packaged app, whose
 * policy allows no fetch of its own files, reads it as it does the rest of
 * its code. A package without a license fails the build.
 */
function openSourceLicenses(): Plugin[] {
  const read = (path: string) => readFileSync(path, "utf8");
  const fonts = fileURLToPath(new URL("./src/features/player/fonts/OFL.txt", import.meta.url));
  const terms = JSON.parse(read(join(shared, "credits/terms.json"))) as {
    terms: (Credit & { apps: string[] })[];
  };
  let imported: Credit[] = [];
  return [
    {
      ...(license({
        thirdParty: {
          allow: { test: (d) => d.license !== null, failOnUnlicensed: true, failOnViolation: true },
          output: (dependencies: Dependency[]) => {
            imported = dependencies.map((d) => ({
              name: d.name ?? "",
              version: d.version ?? undefined,
              license: d.license ?? "",
              text: [d.licenseText, d.noticeText].filter(Boolean).join("\n\n") || undefined,
            }));
          },
        },
      }) as unknown as Plugin),
      apply: "build",
    },
    {
      name: "kivo-licenses",
      apply: "build",
      generateBundle(_, bundle) {
        const names = new Set(imported.map((c) => c.name));
        const styled = new Set(
          Object.values(bundle)
            .flatMap((file) => (file.type === "asset" ? file.originalFileNames : []))
            .map(packageOf)
            .filter((name): name is string => name !== undefined && !names.has(name)),
        );
        const credits: Credit[] = [
          ...[...imported, ...[...styled].map(stylesheetPackage)].sort((a, b) => a.name.localeCompare(b.name)),
          { name: "Kivo Fallback (Arimo, Noto Sans JP)", license: "OFL-1.1", text: read(fonts) },
          ...terms.terms
            .filter((t) => t.apps.includes("web"))
            .map(({ name, license, url, text }) => ({
              name,
              license,
              url,
              text: text ? read(join(shared, "credits", text)) : undefined,
            })),
        ];
        this.emitFile({
          type: "asset",
          fileName: "licenses.js",
          source: `export default ${JSON.stringify(credits)};\n`,
        });
      },
    },
  ];
}

export default defineConfig({
  // Relative asset URLs: the server sets <base href> to its base path, so the
  // same build works at / and behind a reverse proxy at /kivo. The packaged
  // app's page has no <base href>, and its URLs resolve beside it.
  base: "./",
  define: {
    "import.meta.env.VITE_KIVO_BUILD": JSON.stringify(buildTime),
    ...(packaged ? { "import.meta.env.VITE_KIVO_APP_VERSION": JSON.stringify(packageVersion()) } : {}),
  },
  plugins: [
    lingui({ macroTransform: true }),
    react(),
    babel({ presets: [reactCompilerPreset()] }),
    tailwindcss(),
    ...(tv ? [tvSplash()] : []),
    ...(packaged ? [packagedPage()] : []),
    ...(tv && !packaged ? [packagedOnly()] : []),
    ...(process.env.KIVO_MOCK === "1" ? [mockWorker()] : []),
    openSourceLicenses(),
  ],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)), "@shared": shared },
  },
  publicDir: "public",
  server: {
    proxy: { "/api": { target: kivoUrl, ws: true } },
    fs: { allow: [".", shared] },
  },
  build: {
    outDir: packaged ? "dist-packaged" : "dist",
    // Old TV engines. Tizen 8 (2024 TVs) is about Chromium 108 and Tizen 9 is
    // Chromium 120 per Samsung's web engine spec, so 108 covers both. CSS is
    // not downlevelled.
    ...(tv ? { target: "chrome108", cssTarget: "chrome108" } : {}),
    assetsDir: "assets",
    // hls.js, about 575 kB and loaded only for an HLS play, is the largest chunk.
    chunkSizeWarningLimit: 600,
    rolldownOptions: {
      // Norigin starts nothing as it loads, so the web build, which never
      // uses it (src/tv/norigin.ts), drops it whole.
      treeshake: { moduleSideEffects: (id) => !id.includes("/@noriginmedia/") },
      output: {
        // React changes least often, so it gets its own long-cached chunk.
        codeSplitting: {
          groups: [{ name: "react", test: /node_modules[\\/](react|react-dom|scheduler)[\\/]/ }],
        },
      },
    },
  },
  test: {
    restoreMocks: true,
    projects: [
      {
        // Screens and the pieces they use: jsdom, with MSW serving the API.
        extends: true,
        test: {
          name: "dom",
          environment: "jsdom",
          setupFiles: ["./src/test/setup.ts"],
          include: ["src/**/*.test.{ts,tsx}"],
          exclude: ["src/theme/**", "src/**/*.tv.test.{ts,tsx}", "src/**/*.packaged.test.{ts,tsx}"],
        },
      },
      {
        // The same, as the TV build: the flag is a constant, so its tests run apart.
        extends: true,
        test: {
          name: "tv",
          environment: "jsdom",
          setupFiles: ["./src/test/setup.ts", "./src/test/tvSetup.ts"],
          include: ["src/**/*.tv.test.{ts,tsx}"],
          exclude: ["src/**/*.packaged.test.{ts,tsx}"],
          env: { VITE_KIVO_TV: "1" },
        },
      },
      {
        // The packaged TV app: the TV build with the packaged flag too, and a
        // kept server, which its requests can't go out without.
        extends: true,
        test: {
          name: "packaged",
          environment: "jsdom",
          setupFiles: ["./src/test/packagedSetup.ts", "./src/test/setup.ts", "./src/test/tvSetup.ts"],
          include: ["src/**/*.packaged.test.{ts,tsx}"],
          env: { VITE_KIVO_TV: "1", VITE_KIVO_PACKAGED: "1" },
        },
      },
      {
        // Build-time checks: tokens, stylesheet layers, the style lint.
        extends: true,
        test: {
          name: "node",
          environment: "node",
          include: ["src/theme/**/*.test.ts", "scripts/**/*.test.ts"],
        },
      },
    ],
  },
});
