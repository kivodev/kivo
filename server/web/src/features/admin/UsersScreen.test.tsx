import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { locationOf, renderApp, screen } from "@/test/render";
import { http, mockId, mockIds, onRequest, problem, sent, serveMockApi, server } from "@/test/server";

const ben = mockId(7101);

describe("Admin Users", () => {
  it("lists each user with its libraries and badges, and the pending invites", async () => {
    serveMockApi();
    renderApp("/admin/users");
    const anna = (await screen.findByRole("link", { name: "anna" })).closest<HTMLElement>('[data-part="user"]');
    expect(anna).toHaveTextContent("Admin");
    expect(anna).toHaveTextContent("All libraries");
    const benRow = screen.getByRole("link", { name: "ben" }).closest<HTMLElement>('[data-part="user"]');
    expect(benRow).toHaveTextContent("Movies");
    expect(benRow).not.toHaveTextContent("Admin");
    const invite = document.querySelector<HTMLElement>('[data-part="invite"]');
    expect(invite).toHaveTextContent(/^All libraries · Expires [A-Z][a-z]{2} \d{1,2}Revoke$/);
  });

  it("adds a user once libraries are chosen, then opens their page", async () => {
    const api = serveMockApi();
    onRequest("post", "/users", () => {
      api.users.push({
        id: mockId(7102),
        username: "cara",
        admin: false,
        disabled: false,
        onlyAdmin: false,
        libraries: { all: false, libraryIds: [mockIds.movies] },
        limits: {},
        parental: { blockUnrated: false },
        createdAt: "2026-10-01T00:00:00Z",
      });
    });
    const { user, router } = renderApp("/admin/users");
    await user.click(await screen.findByRole("button", { name: "Add user" }));
    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByRole("textbox", { name: "Username" }), "cara");
    await user.type(within(dialog).getByLabelText("Password"), "a good password");
    const save = within(dialog).getByRole("button", { name: "Add user" });
    expect(save).toBeDisabled();
    expect(within(dialog).getByRole("switch", { name: "All libraries" })).not.toBeChecked();
    await user.click(within(dialog).getByRole("checkbox", { name: "Movies" }));
    expect(save).toBeEnabled();
    await user.click(save);

    await waitFor(() => expect(locationOf(router)).toBe(`/admin/users/${mockId(7102)}`));
    expect(sent("POST", "/users")[0]?.body).toEqual({
      username: "cara",
      password: "a good password",
      libraries: { all: false, libraryIds: [mockIds.movies] },
      limits: {},
    });
  });

  it("creates an invite link under the server's own address", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/users");
    await user.click(await screen.findByRole("button", { name: "Create invite link" }));
    const dialog = await screen.findByRole("dialog");
    const create = within(dialog).getByRole("button", { name: "Create link" });
    expect(create).toBeDisabled();
    await user.click(within(dialog).getByRole("switch", { name: "All libraries" }));
    await user.click(create);

    const link = await within(dialog).findByRole("textbox", { name: "Invite link" });
    expect(link).toHaveValue(new URL("invite/invite-token", document.baseURI).href);
    expect(dialog).toHaveTextContent("Anyone with this link can create one account.");
    expect(dialog).toHaveTextContent("Uses this browser's address.");
    const body = sent("POST", "/invites")[0]?.body as { libraries: unknown; expiresAt: string };
    expect(body.libraries).toEqual({ all: true, libraryIds: [] });
    // Seven days by default.
    expect(new Date(body.expiresAt).getTime() - Date.now()).toBeGreaterThan(6.9 * 24 * 3600 * 1000);
  });

  it("creates an invite link under the Public URL when there is one, and keeps it open until Done", async () => {
    const api = serveMockApi();
    api.settings = { ...api.settings, publicUrl: "https://kivo.example.com/media" };
    const { user } = renderApp("/admin/users");
    await user.click(await screen.findByRole("button", { name: "Create invite link" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("switch", { name: "All libraries" }));
    await user.click(within(dialog).getByRole("button", { name: "Create link" }));
    const link = await within(dialog).findByRole("textbox", { name: "Invite link" });
    expect(link).toHaveValue("https://kivo.example.com/media/invite/invite-token");
    expect(dialog).not.toHaveTextContent("Uses this browser's address.");
    expect(dialog).toHaveTextContent("It won't be shown again.");
    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Done" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("shows an invite's limits on its line when it has them", async () => {
    serveMockApi((api) => {
      const invite = api.invites[0];
      if (invite) invite.limits = { maxStreams: 2 };
    });
    renderApp("/admin/users");
    await screen.findByRole("heading", { name: "Pending invites" });
    const invite = document.querySelector<HTMLElement>('[data-part="invite"]');
    expect(invite).toHaveTextContent(/^All libraries · Up to 2 streams · Expires [A-Z][a-z]{2} \d{1,2}Revoke$/);
  });

  it("revokes an invite after a confirmation", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/users");
    await user.click(await screen.findByRole("button", { name: "Revoke" }));
    await user.click(await screen.findByRole("button", { name: "Revoke" }));
    await waitFor(() => expect(sent("DELETE", `/invites/${mockId(7201)}`)).toHaveLength(1));
  });
});

describe("A user's page", () => {
  it("saves access, limits and parental controls with one Save, sending what changed", async () => {
    serveMockApi();
    const { user } = renderApp(`/admin/users/${ben}`);
    expect(await screen.findByRole("link", { name: "Users ›" })).toHaveAttribute("href", "/admin/users");
    const page = document.querySelector<HTMLElement>('[data-part="admin-page"]') as HTMLElement;
    expect(within(page).getByRole("link", { name: "Watch history" })).toHaveAttribute(
      "href",
      `/admin/history?userId=${ben}`,
    );
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
    const limits = screen.getByRole("region", { name: "Limits" });
    await user.click(within(limits).getByRole("button", { name: /Simultaneous streams/ }));
    await user.click(await screen.findByRole("option", { name: "No limit" }));
    const parental = screen.getByRole("region", { name: "Parental controls" });
    await user.click(within(parental).getByRole("switch", { name: "Block unrated items" }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(sent("PATCH", `/users/${ben}`)).toHaveLength(1));
    expect(sent("PATCH", `/users/${ben}`)[0]?.body).toEqual({
      limits: {},
      parental: { maxAge: 13, blockUnrated: false },
    });
  });

  it("renames the user, trimmed, and says when the name is taken", async () => {
    serveMockApi();
    const { user } = renderApp(`/admin/users/${ben}`);
    const name = await screen.findByRole("textbox", { name: "Username" });
    expect(name).toHaveValue("ben");
    await user.clear(name);
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    await user.type(name, " benji ");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(sent("PATCH", `/users/${ben}`)[0]?.body).toEqual({ username: "benji" }));

    server.use(http.patch("/api/v1/users/{id}", () => problem(409, "nameTaken")));
    await user.clear(name);
    await user.type(name, "anna");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText("That username is taken.")).toBeInTheDocument();
  });

  it("asks before making someone an admin", async () => {
    serveMockApi();
    const { user } = renderApp(`/admin/users/${ben}`);
    await user.click(await screen.findByRole("switch", { name: "Admin" }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Make ben an admin?" });
    expect(sent("PATCH", `/users/${ben}`)).toHaveLength(0);
    await user.click(within(confirm).getByRole("button", { name: "Make admin and save" }));
    await waitFor(() => expect(sent("PATCH", `/users/${ben}`)[0]?.body).toEqual({ admin: true }));
  });

  it("shows the save bar while something differs, and Discard puts the user's settings back", async () => {
    serveMockApi();
    const { user } = renderApp(`/admin/users/${ben}`);
    const block = await screen.findByRole("switch", { name: "Block unrated items" });
    expect(block).toBeChecked();
    await user.click(block);
    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Discard" }));
    expect(block).toBeChecked();
    expect(screen.queryByText("Unsaved changes")).not.toBeInTheDocument();
  });

  it("waits for a library before Save, and keeps a stream limit over 10 as a choice", async () => {
    const api = serveMockApi();
    const benUser = api.users.find((u) => u.id === ben);
    if (!benUser) throw new Error("no ben");
    benUser.limits = { maxStreams: 12 };
    const { user } = renderApp(`/admin/users/${ben}`);
    expect(await screen.findByRole("button", { name: /Simultaneous streams/ })).toHaveTextContent("12");
    await user.click(screen.getByRole("checkbox", { name: "Movies" }));
    expect(screen.getByText("Select at least one library.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("asks before disabling someone", async () => {
    serveMockApi();
    const { user } = renderApp(`/admin/users/${ben}`);
    await user.click(await screen.findByRole("button", { name: "Disable" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Disable ben?" });
    await user.click(within(confirm).getByRole("button", { name: "Disable" }));
    await waitFor(() => expect(sent("PATCH", `/users/${ben}`)[0]?.body).toEqual({ disabled: true }));
  });

  it("saves with Enter in a field, and the save bar goes once the user is saved", async () => {
    serveMockApi();
    const { user } = renderApp(`/admin/users/${ben}`);
    const name = await screen.findByRole("textbox", { name: "Username" });
    await user.type(name, "ji{Enter}");
    await waitFor(() => expect(sent("PATCH", `/users/${ben}`)[0]?.body).toEqual({ username: "benji" }));
    await waitFor(() => expect(screen.queryByText("Unsaved changes")).not.toBeInTheDocument());
    expect(name).toHaveValue("benji");
  });

  it("lists their devices, and signs one or all of them out once you confirm", async () => {
    // None of ben's sessions is this browser's.
    const api = serveMockApi((api) => {
      for (const s of api.sessions) s.current = false;
    });
    const { user } = renderApp(`/admin/users/${ben}`);
    const devices = await screen.findByRole("region", { name: "Devices" });
    await waitFor(() => expect(within(devices).getAllByRole("listitem")).toHaveLength(api.sessions.length));
    expect(sent("GET", "/sessions").at(-1)?.url.searchParams.get("userId")).toBe(ben);
    expect(within(devices).queryByRole("link", { name: "Link a device" })).not.toBeInTheDocument();
    const first = api.sessions[0];
    if (!first) throw new Error("no sessions");
    await user.click(within(devices).getByRole("button", { name: `Sign out ${first.name}` }));
    const one = await screen.findByRole("alertdialog", { name: `Sign out ${first.name}?` });
    await user.click(within(one).getByRole("button", { name: "Sign out" }));
    await waitFor(() => expect(sent("DELETE", `/sessions/${first.id}`)).toHaveLength(1));

    await user.click(within(devices).getByRole("button", { name: "Sign out everywhere" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Sign ben out everywhere?" });
    await user.click(within(confirm).getByRole("button", { name: "Sign out everywhere" }));
    await waitFor(() => expect(sent("DELETE", "/sessions/{id}")).toHaveLength(1 + api.sessions.length));
    expect(sent("DELETE", "/sessions")).toHaveLength(0);
  });

  it("won't take admin away from, disable or delete yourself", async () => {
    serveMockApi();
    renderApp(`/admin/users/${mockId(7001)}`);
    expect(await screen.findByRole("switch", { name: "Admin" })).toBeDisabled();
    // The heading has the Admin badge beside the name. The tab title has
    // the name alone.
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("annaAdmin");
    await waitFor(() => expect(document.title).toBe("anna – Kivo"));
    expect(screen.getByRole("button", { name: "Disable" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Delete" })).toBeDisabled();
    expect(screen.getAllByText("Not available for your own account.").length).toBeGreaterThan(0);
  });

  it("deletes another user after a confirmation, then goes back to the list", async () => {
    serveMockApi();
    const { user, router } = renderApp(`/admin/users/${ben}`);
    await user.click(await screen.findByRole("button", { name: "Delete" }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(locationOf(router)).toBe("/admin/users"));
    expect(sent("DELETE", `/users/${ben}`)).toHaveLength(1);
  });

  it("sets a password from Account", async () => {
    serveMockApi();
    const { user } = renderApp(`/admin/users/${ben}`);
    const account = await screen.findByRole("region", { name: "Account" });
    // An unsaved edit stays unsaved, because the dialog's form is separate
    // from the page's.
    await user.click(screen.getByRole("switch", { name: "Block unrated items" }));
    await user.click(within(account).getByRole("button", { name: "Set password…" }));
    const dialog = await screen.findByRole("dialog", { name: "Set ben's password" });
    await user.type(within(dialog).getByLabelText(/New password/), "a new password");
    await user.click(within(dialog).getByRole("button", { name: "Set password" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() =>
      expect(sent("POST", `/users/${ben}/set-password`)[0]?.body).toEqual({ password: "a new password" }),
    );
    expect(sent("PATCH", `/users/${ben}`)).toHaveLength(0);
  });
});
