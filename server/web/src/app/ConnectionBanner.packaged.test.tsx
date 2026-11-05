import { waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { locationOf, renderApp, screen } from "@/test/render";
import { sendEvent, serveMockApi } from "@/test/server";
import { setPackagedToken } from "@/tv/packaged";

vi.mock("@/tv/exit", () => ({ exitApp: vi.fn() }));

// The packaged app would only run its own code again, so a server updated
// while it runs is no reason to offer a reload.
it("offers no reload when the server is updated while the packaged app runs", async () => {
  const api = serveMockApi((api) => {
    api.server.version = "2.0.0";
    api.snapshot = { ...api.snapshot, serverVersion: "2.0.0" };
  });

  setPackagedToken("app-token");
  const { router } = renderApp("/");
  await waitFor(() => expect(document.activeElement?.getAttribute("data-tv-focus-key")).toMatch(/^card:/));
  sendEvent({ ...api.snapshot, serverVersion: "2.1.0" } as unknown as Parameters<typeof sendEvent>[0]);
  await new Promise((resolve) => setTimeout(resolve, 50));
  expect(screen.queryByText("The server was updated. Reload to use the new version.")).not.toBeInTheDocument();
  expect(locationOf(router)).toBe("/");
});
