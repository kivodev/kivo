import { fireEvent, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { locationOf, renderApp, screen } from "@/test/render";
import { http, problem, serveMockApi, server } from "@/test/server";
import { packagedServer } from "@/tv/packaged";

vi.mock("@/tv/exit", () => ({ exitApp: vi.fn(), changeServer: vi.fn() }));

describe("The packaged app's load error", () => {
  it("offers Change server beside Try again, which opens the address screen with the kept address", async () => {
    serveMockApi();
    server.use(http.get("/api/v1/setup", () => problem(503, "notReady")));
    const { router, user } = renderApp("/");
    expect(await screen.findByRole("button", { name: "Try again" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Change server" }));
    expect(await screen.findByLabelText("Address")).toHaveValue("http://kivo.test");
    expect(locationOf(router)).toBe("/server");
    expect(packagedServer()?.address).toBe("http://kivo.test");
    // Back returns to the page that failed, rather than asking to exit.
    fireEvent.keyDown(window, { key: "XF86Back" });
    await waitFor(() => expect(screen.queryByLabelText("Address")).not.toBeInTheDocument());
    expect(screen.queryByRole("alertdialog", { name: "Exit Kivo?" })).not.toBeInTheDocument();
  });
});
