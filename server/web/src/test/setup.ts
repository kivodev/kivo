import "@testing-library/jest-dom/vitest";
import { cleanup, configure } from "@testing-library/react";
import { destroyAnnouncer } from "react-aria/private/live-announcer/LiveAnnouncer";
import { afterAll, afterEach, beforeAll, vi } from "vitest";
import "@/i18n";
import { installMediaElement } from "./media";
import { server } from "./server";

// React's act environment, which React Testing Library turns off inside waitFor.
(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// A screen can take more than the default second to render on a
// shared CI runner.
configure({ asyncUtilTimeout: 5000 });

// jsdom lacks these browser APIs. The app only needs them to exist.
class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver;
// No idle callback. Without this, the capability report that the app
// measures when idle falls back to a one-second timer. That timer can fire
// after a test file's jsdom is torn down ("document is not defined"). Play
// measures the report anyway.
globalThis.requestIdleCallback ??= (() => 0) as unknown as typeof requestIdleCallback;
window.scrollTo = vi.fn() as unknown as typeof window.scrollTo;
Element.prototype.scrollTo = vi.fn() as unknown as typeof Element.prototype.scrollTo;
// jsdom has no canvas, so blurhash placeholders are skipped.
HTMLCanvasElement.prototype.getContext = (() => null) as unknown as typeof HTMLCanvasElement.prototype.getContext;
// jsdom has no media pipeline, so video elements load at once (see
// media.ts).
installMediaElement();

beforeAll(() => server.listen({ onUnhandledFrame: "error" }));
afterEach(async () => {
  cleanup();
  // React Aria's live announcer outlives the render, and its region stays
  // in the body. Its first announcement, if made inside waitFor or a user
  // event where the act environment is off, waits on a 100 ms timer. That
  // timer can fire after the file's jsdom is torn down ("document is not
  // defined"). Destroying the announcer drops the region, and the timer
  // then finds no announcer.
  destroyAnnouncer();
  // Each test gets its own browser storage, such as the state a library
  // was left in.
  localStorage.clear();
  // What unmounting sends, such as the player's stop, goes out before the
  // next test starts counting requests.
  await new Promise((resolve) => setTimeout(resolve));
  server.resetHandlers();
});
afterAll(() => server.close());
