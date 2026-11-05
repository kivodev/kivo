import { waitFor } from "@testing-library/react";
import { HttpResponse, http as untyped } from "msw";
import { describe, expect, it } from "vitest";
import { renderApp, screen } from "@/test/render";
import { closeEvents, serveMockApi, server } from "@/test/server";

describe("The connection notice on TV", () => {
  it("floats over the screen without a control, so it never moves the page or takes focus", async () => {
    serveMockApi();
    server.use(untyped.get("*/api/v1/server", () => HttpResponse.error()));
    renderApp("/");
    await waitFor(() => expect(document.activeElement?.getAttribute("data-tv-focus-key")).toMatch(/^card:/));
    const focused = document.activeElement;
    await waitFor(() => expect(closeEvents(1011)).toBeGreaterThan(0));
    const notice = await screen.findByText("Can't reach Living room. Reconnecting…");
    expect(notice.closest("[data-part=connection-notice]")).toHaveClass("fixed");
    expect(document.activeElement).toBe(focused);
    expect(screen.queryByRole("button", { name: "Retry now" })).not.toBeInTheDocument();
  });
});
