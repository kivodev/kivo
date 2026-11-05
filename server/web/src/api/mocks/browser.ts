import { bypass, delay, HttpResponse, http, ws } from "msw";
import { setupWorker } from "msw/browser";
import { createMockApi, scenarios } from "./fixtures";
import { closeEvents, createEventsHandler, createHandlers } from "./handlers";

/**
 * The playable scenario's media (mock-public/mock-media), answered a range
 * at a time as a server answers a video's requests. Passed through the
 * worker, a range comes back as the whole file, and the video can't seek.
 */
function mediaHandler() {
  const files = new Map<string, Promise<ArrayBuffer>>();
  return http.get("/mock-media/:name", async ({ request }) => {
    const { pathname } = new URL(request.url);
    let file = files.get(pathname);
    if (!file) {
      file = fetch(bypass(request.url)).then((r) => {
        if (!r.ok) throw new Error(`${pathname}: ${r.status}`);
        return r.arrayBuffer();
      });
      // A failed fetch, such as while the dev server restarts, is tried again.
      file.catch(() => files.delete(pathname));
      files.set(pathname, file);
    }
    const bytes = await file;
    const size = bytes.byteLength;
    const range = /^bytes=(\d+)-(\d*)$/.exec(request.headers.get("range") ?? "");
    const start = Number(range?.[1] ?? 0);
    const end = range?.[2] ? Math.min(Number(range[2]), size - 1) : size - 1;
    if (start > end) {
      return new HttpResponse(null, { status: 416, headers: { "Content-Range": `bytes */${size}` } });
    }
    return new HttpResponse(bytes.slice(start, end + 1), {
      status: range ? 206 : 200,
      headers: {
        "Content-Type": "video/mp4",
        "Accept-Ranges": "bytes",
        "Content-Length": String(end - start + 1),
        ...(range ? { "Content-Range": `bytes ${start}-${end}/${size}` } : {}),
      },
    });
  });
}

/**
 * Serves the API from MSW in the browser, for `npm run dev:mock`. It serves
 * the sample server with an admin signed in. ?mock= names scenarios to
 * apply, comma-separated, such as ?mock=setup for a fresh server or
 * ?mock=scanning,unreachable for the admin pages at work. ?delay=400
 * answers every request that many milliseconds late, to show the loading
 * states. Changes aren't kept, so the sample answers stay as they are.
 *
 * For the browser tests, window.kivoMock.serverGone() takes the server
 * away as the app runs: its event socket drops, each new one is refused,
 * and asking whether the server is there gets no answer.
 */
export async function startMockWorker(): Promise<void> {
  const api = createMockApi();
  const params = new URLSearchParams(window.location.search);
  for (const name of params.get("mock")?.split(",") ?? []) {
    if (Object.hasOwn(scenarios, name)) scenarios[name as keyof typeof scenarios](api);
  }
  const late = Number(params.get("delay") ?? 0);
  const delayed = late > 0 ? [http.all("*/api/v1/*", () => delay(late))] : [];
  const worker = setupWorker(...delayed, ...createHandlers(api), createEventsHandler(api), mediaHandler());
  await worker.start({ onUnhandledFrame: "bypass" });
  const serverGone = () => {
    worker.use(
      http.get("*/api/v1/server", () => HttpResponse.error()),
      ws.link("*/api/v1/events").addEventListener("connection", (event) => {
        event.stopImmediatePropagation();
        event.client.close(1011);
      }),
    );
    closeEvents(1011);
  };
  Object.assign(window, { kivoMock: { serverGone } });
}
