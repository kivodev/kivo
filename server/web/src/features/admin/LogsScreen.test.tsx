import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderApp, screen } from "@/test/render";
import { sent, serveMockApi } from "@/test/server";

const messages = () =>
  screen
    .queryAllByText(/^(kivo|scan|metadata|api): /, { selector: "[data-part=log-line] span" })
    .map((s) => s.textContent);

const lastQuery = () => sent("GET", "/logs").at(-1)?.url.searchParams;

describe("Logs", () => {
  it("is in the admin sidebar, and shows the lines newest last, with their attributes", async () => {
    serveMockApi();
    const { user } = renderApp("/admin");
    await user.click(
      within(await screen.findByRole("navigation", { name: "Admin" })).getByRole("link", { name: "Logs" }),
    );
    expect(await screen.findByRole("heading", { name: "Logs" })).toBeInTheDocument();
    await waitFor(() =>
      expect(messages()).toEqual([
        "kivo: starting",
        "scan: finished",
        "scan: folder unreachable; nothing in it changes",
        "metadata: provider unavailable",
      ]),
    );
    expect(lastQuery()?.get("level")).toBe("info");
    expect(screen.getByText(/root=\/mnt\/user\/TV err=no such file or directory/)).toBeInTheDocument();
    const error = screen.getByText("metadata: provider unavailable").closest("[data-part=log-line]");
    expect(error).toHaveAttribute("data-level", "error");
  });

  it("asks for a level and a search, and says when no line matches", async () => {
    const api = serveMockApi();
    const { user } = renderApp("/admin/logs");
    await user.click(await screen.findByRole("button", { name: /Level/ }));
    await user.click(await screen.findByRole("option", { name: "Errors" }));
    await waitFor(() => expect(lastQuery()?.get("level")).toBe("error"));

    api.logs = [];
    await user.type(screen.getByRole("searchbox", { name: "Search the log" }), "nothing like this");
    await waitFor(() => expect(lastQuery()?.get("q")).toBe("nothing like this"));
    expect(await screen.findByText("No log lines match.")).toBeInTheDocument();
  });

  it("says when Debug lines appear", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/logs");
    expect(screen.queryByText("Debug lines appear when the server's log level is debug.")).not.toBeInTheDocument();
    await user.click(await screen.findByRole("button", { name: /Level/ }));
    await user.click(await screen.findByRole("option", { name: "Debug" }));
    expect(await screen.findByText("Debug lines appear when the server's log level is debug.")).toBeInTheDocument();
  });

  it("downloads the log as a file", async () => {
    serveMockApi();
    renderApp("/admin/logs");
    const link = await screen.findByRole("link", { name: "Download" });
    expect(link.getAttribute("href")).toMatch(/\/api\/v1\/logs\/file$/);
    expect(link).toHaveAttribute("download");
  });
});
