import { waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderApp, screen } from "@/test/render";
import { mockId, onRequest, sent, serveMockApi } from "@/test/server";

describe("Admin API keys", () => {
  it("lists each key with when it was created and last used", async () => {
    serveMockApi();
    renderApp("/admin/settings/api-keys");
    expect(await screen.findByRole("heading", { name: "API keys" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "API keys" })).toHaveAttribute("aria-current", "page");
    const used = screen.getByText("Home Assistant").closest("li");
    expect(used).toHaveTextContent("Last used");
    expect(screen.getByText("Overseerr").closest("li")).toHaveTextContent("Never used");
  });

  it("says so when there are none", async () => {
    serveMockApi((api) => {
      api.apiKeys = [];
    });
    renderApp("/admin/settings/api-keys");
    expect(await screen.findByText("No API keys")).toBeInTheDocument();
  });

  it("creates a key and shows it once, to copy", async () => {
    const api = serveMockApi();
    onRequest("post", "/api-keys", () => {
      api.apiKeys.unshift({ id: mockId(7303), name: "Tautulli", createdAt: new Date().toISOString() });
    });
    const { user } = renderApp("/admin/settings/api-keys");
    await user.click(await screen.findByRole("button", { name: "Create key" }));
    const dialog = await screen.findByRole("dialog");
    const create = within(dialog).getByRole("button", { name: "Create key" });
    expect(create).toBeDisabled();
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), " Tautulli ");
    await user.click(create);

    expect(await within(dialog).findByRole("textbox", { name: "API key" })).toHaveValue("api-key-token");
    expect(dialog).toHaveTextContent("This key has admin access.");
    expect(dialog).toHaveTextContent("It won't be shown again.");
    expect(sent("POST", "/api-keys")[0]?.body).toEqual({ name: "Tautulli" });
    await user.click(within(dialog).getByRole("button", { name: "Copy" }));
    expect(await navigator.clipboard.readText()).toBe("api-key-token");
    expect(within(dialog).getByRole("button", { name: "Copied" })).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Done" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(await screen.findByText("Tautulli")).toBeInTheDocument();
    expect(screen.queryByText("api-key-token")).not.toBeInTheDocument();
  });

  it("keeps the new key open until Done, and selects it where the clipboard is blocked", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/settings/api-keys");
    await user.click(await screen.findByRole("button", { name: "Create key" }));
    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "Tautulli");
    await user.click(within(dialog).getByRole("button", { name: "Create key" }));
    const key = await within(dialog).findByRole("textbox", { name: "API key" });

    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    vi.spyOn(navigator.clipboard, "writeText").mockRejectedValueOnce(new Error("not allowed"));
    await user.click(within(dialog).getByRole("button", { name: "Copy" }));
    expect(await within(dialog).findByText("Press Ctrl+C to copy.")).toBeInTheDocument();
    expect((key as HTMLInputElement).selectionEnd).toBe("api-key-token".length);
  });

  it("asks before Escape discards a name being typed", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/settings/api-keys");
    await user.click(await screen.findByRole("button", { name: "Create key" }));
    const dialog = await screen.findByRole("dialog");
    const name = within(dialog).getByRole("textbox", { name: "Name" });
    await user.type(name, "Tautulli");
    await user.keyboard("{Escape}");
    const confirm = await screen.findByRole("alertdialog", { name: "Discard changes?" });
    await user.click(within(confirm).getByRole("button", { name: "Cancel" }));
    expect(name).toHaveValue("Tautulli");
    await user.click(name);
    await user.keyboard("{Escape}");
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Discard" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("revokes a key after a confirmation", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/settings/api-keys");
    const row = (await screen.findByText("Overseerr")).closest("li") as HTMLElement;
    await user.click(within(row).getByRole("button", { name: "Revoke Overseerr" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Revoke Overseerr?" });
    expect(confirm).toHaveTextContent("Apps and scripts using this key stop working immediately.");
    await user.click(within(confirm).getByRole("button", { name: "Revoke" }));
    await waitFor(() => expect(sent("DELETE", "/api-keys/{id}")).toHaveLength(1));
    expect(sent("DELETE", "/api-keys/{id}")[0]?.url.pathname).toMatch(new RegExp(`/api-keys/${mockId(7302)}$`));
  });
});
