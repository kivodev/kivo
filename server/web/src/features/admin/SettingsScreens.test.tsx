import { waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { MockApi } from "@/api/mocks/api";
import { renderApp, screen } from "@/test/render";
import { http, onRequest, problem, sent, serveMockApi, server } from "@/test/server";

/** Keeps what a settings save sends, as the server does. */
function keepSettings(api: MockApi) {
  onRequest("patch", "/settings", (body) => {
    api.settings = { ...api.settings, ...(body as object) };
  });
}

describe("Admin settings", () => {
  it("lists the admin pages in the sidebar's groups", async () => {
    serveMockApi();
    renderApp("/admin/settings/general");
    const nav = await screen.findByRole("navigation", { name: "Admin" });
    expect(nav.textContent).toBe(
      [
        "Overview",
        "ManageLibrariesUsersWatch history",
        "MaintenanceActivityTasksLogs",
        "SettingsGeneralLibraryTranscodingNetworkAPI keys",
      ].join(""),
    );
  });

  it("has no Background work page, its settings being on Tasks", async () => {
    serveMockApi();
    renderApp("/admin/settings/background-work");
    expect(await screen.findByRole("heading", { name: "This page doesn't exist." })).toBeInTheDocument();
  });

  it("General saves the server's name and provider keys, and says nothing when it works", async () => {
    const api = serveMockApi();
    keepSettings(api);
    const { user } = renderApp("/admin/settings/general");
    const name = await screen.findByRole("textbox", { name: "Server name" });
    expect(name).toHaveValue("Living room");
    expect(screen.queryByRole("textbox", { name: "Played threshold" })).not.toBeInTheDocument();

    await user.clear(name);
    await user.type(name, "Basement");
    await user.type(screen.getByRole("textbox", { name: "OpenSubtitles API key" }), "os-key");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(sent("PATCH", "/settings")[0]?.body).toEqual({
        serverName: "Basement",
        openSubtitlesKey: "os-key",
        openSubtitlesUsername: "",
      }),
    );
    await waitFor(() => expect(screen.queryByText("Unsaved changes")).not.toBeInTheDocument());
    expect(screen.queryByText("Saved.")).not.toBeInTheDocument();
  });

  it("General sends the OpenSubtitles password only when a new one is typed", async () => {
    const api = serveMockApi();
    api.settings = { ...api.settings, openSubtitlesUsername: "anna", openSubtitlesPasswordSet: true };
    const { user } = renderApp("/admin/settings/general");
    const password = await screen.findByLabelText("OpenSubtitles password");
    expect(password).toHaveValue("");
    expect(password).toHaveAccessibleDescription("Saved. Enter a new password to replace it.");
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();

    await user.type(password, "hunter22");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(sent("PATCH", "/settings")[0]?.body).toMatchObject({ openSubtitlesPassword: "hunter22" }),
    );
    await waitFor(() => expect(password).toHaveValue(""));
  });

  it("shows the save bar only with a change, and Discard puts the saved settings back", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/settings/network");
    const url = await screen.findByRole("textbox", { name: "Public URL" });
    expect(screen.queryByText("Unsaved changes")).not.toBeInTheDocument();
    await user.type(url, "https://kivo.example.com");
    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Discard" }));
    expect(url).toHaveValue("");
    expect(screen.queryByText("Unsaved changes")).not.toBeInTheDocument();
    expect(sent("PATCH", "/settings")).toHaveLength(0);
  });

  it("keeps the save bar after a refused save, with what the field takes on its row", async () => {
    serveMockApi();
    server.use(
      http.patch("/api/v1/settings", () =>
        problem(400, "invalidRequest", { errors: [{ field: "/publicUrl", code: "invalid" }] }),
      ),
    );
    const { user } = renderApp("/admin/settings/network");
    const url = await screen.findByRole("textbox", { name: "Public URL" });
    await user.type(url, "ftp://kivo.example.com{Enter}");
    const hint = "Use a full address starting with http:// or https://.";
    expect(await screen.findByText(hint)).toBeInTheDocument();
    expect(url).toHaveAccessibleDescription(expect.stringContaining(hint));
    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  });

  it("Library saves file watching, the metadata locale and the played threshold", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/settings/library");
    const watching = await screen.findByRole("switch", { name: "Scan when files change" });
    expect(watching).toBeChecked();
    expect(screen.getByRole("button", { name: /Metadata language/ })).toHaveTextContent("American English");
    expect(screen.getByRole("button", { name: /Certification country/ })).toHaveTextContent("United States");
    expect(screen.getByRole("textbox", { name: "Played threshold" })).toHaveValue("90%");

    await user.click(watching);
    const threshold = screen.getByRole("textbox", { name: "Played threshold" });
    await user.clear(threshold);
    await user.type(threshold, "95%");
    await user.tab();
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(sent("PATCH", "/settings")[0]?.body).toEqual({
        fileWatching: false,
        metadataLanguage: "en-US",
        contentRatingCountry: "US",
        playedThreshold: 0.95,
      }),
    );
  });

  it("Network saves the local networks and the public URL", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/settings/network");
    await user.type(await screen.findByRole("textbox", { name: "Local networks" }), "10.0.0.0/8, 192.168.1.0/24");
    await user.type(screen.getByRole("textbox", { name: "Public URL" }), "https://kivo.example.com");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(sent("PATCH", "/settings")[0]?.body).toEqual({
        localNetworks: ["10.0.0.0/8", "192.168.1.0/24"],
        publicUrl: "https://kivo.example.com",
        remoteQualityCapBps: 0,
      }),
    );
  });
});
