import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Task } from "@/api/schemas";
import { renderApp, screen } from "@/test/render";
import { onRequest, sendEvent, sent, serveMockApi } from "@/test/server";

const section = (name: string) => {
  const el = screen.getByRole("heading", { name }).closest("section");
  if (!el) throw new Error(`no section ${name}`);
  return el;
};

/** A row of a list or table, by the text it starts with. */
const rowOf = (within: HTMLElement, start: string) => {
  const row = [...within.querySelectorAll("li, tbody tr")].find((r) => r.textContent?.startsWith(start));
  if (!row) throw new Error(`no row ${start}`);
  return row as HTMLElement;
};

/** A table row's cells, as text. */
const cells = (row: HTMLElement) =>
  within(row)
    .getAllByRole("cell")
    .map((c) => c.textContent);

/** A section's table rows, below its column headers. */
const rows = (section: HTMLElement) => [...section.querySelectorAll("tbody tr")] as HTMLElement[];

/** The mock's task with this ID. */
const taskOf = (tasks: Task[], id: string) => {
  const task = tasks.find((t) => t.id === id);
  if (!task) throw new Error(`no task ${id}`);
  return task;
};

describe("Tasks", () => {
  it("says when the nightly tasks run, then lists Activity, Library and Maintenance", async () => {
    serveMockApi();
    renderApp("/admin/tasks");
    await screen.findByRole("heading", { name: "Library" });
    expect(document.querySelector('[data-part="nightly-window"]')).toHaveTextContent(
      /^Nightly tasks run 2:00\sAM to 6:00\sAM$/,
    );
    expect(screen.queryByRole("button", { name: /Nightly start/ })).not.toBeInTheDocument();
    expect(screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent)).toEqual([
      "Activity",
      "Library",
      "Maintenance",
    ]);
  });

  it("opens Schedule with the nightly window and the files at once, saving each change at once", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/tasks");
    await user.click(await screen.findByRole("button", { name: "Schedule" }));
    const schedule = await screen.findByRole("dialog", { name: "Schedule" });
    const select = (name: string) => within(schedule).getByRole("button", { name: new RegExp(name) });
    expect(select("Nightly start")).toHaveTextContent(/^2:00\sAM/);
    expect(select("Nightly end")).toHaveTextContent(/^6:00\sAM/);
    expect(select("Preview thumbnails at once")).toHaveTextContent(/^2/);
    expect(select("Intro and credits at once")).toHaveTextContent(/^1/);
    expect(schedule).not.toHaveTextContent("CPU");

    const choose = async (name: string, option: RegExp | string, body: object, n: number) => {
      await waitFor(() => expect(select(name)).not.toBeDisabled());
      await user.click(select(name));
      await user.click(await screen.findByRole("option", { name: option }));
      await waitFor(() => expect(sent("PATCH", "/settings")[n]?.body).toEqual(body));
    };
    await choose("Nightly start", /^11:00\sPM$/, { maintenanceStartHour: 23 }, 0);
    await choose("Nightly end", /^5:00\sAM$/, { maintenanceEndHour: 5 }, 1);
    await choose("Preview thumbnails at once", "6", { previewsAtOnce: 6 }, 2);
    await user.click(select("Intro and credits at once"));
    expect(await screen.findByRole("option", { name: "4" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "5" })).not.toBeInTheDocument();
    await user.keyboard("{Escape}");
    await choose("Intro and credits at once", "3", { markersAtOnce: 3 }, 3);
  });

  it("lists the Library tasks with the same columns as Maintenance: last run, next run, progress and the action", async () => {
    serveMockApi();
    renderApp("/admin/tasks");
    await screen.findByRole("heading", { name: "Library" });
    for (const name of ["Library", "Maintenance"]) {
      expect(
        within(section(name))
          .getAllByRole("columnheader")
          .map((h) => h.textContent),
      ).toEqual(["Task", "Last run", "Next run", "Progress", ""]);
    }
    const library = section("Library");
    expect(rows(library).map((r) => cells(r)[0])).toEqual([
      "Scan all libraries",
      "Refresh metadata",
      "Preview thumbnails",
      "Intro and credits",
      "Extract subtitles",
      "Remove missing files",
    ]);
    expect(cells(rowOf(library, "Scan all libraries"))).toEqual([
      "Scan all libraries",
      "3 hours ago",
      expect.stringMatching(/\d:\d\d\s[AP]M$/),
      "Reading files37 of 120",
      "Stop",
    ]);
    expect(cells(rowOf(library, "Refresh metadata")).slice(2)).toEqual(["Nightly", "", "Run now"]);
    const previews = rowOf(library, "Preview thumbnails");
    expect(cells(previews).slice(3)).toEqual(["18,489 left", "Run now"]);
    expect(within(previews).queryByRole("progressbar")).not.toBeInTheDocument();
    expect(within(previews).getByRole("link", { name: "Preview thumbnails" })).toHaveAttribute(
      "href",
      "/admin/tasks/make-previews",
    );
    expect(cells(rowOf(library, "Intro and credits")).slice(3)).toEqual(["Analyzing312 of 1,516", "Stop"]);
    expect(within(rowOf(library, "Remove missing files")).queryByRole("link")).not.toBeInTheDocument();

    expect(rows(section("Maintenance")).map((r) => cells(r)[0])).toEqual([
      "Clean up preview thumbnails",
      "Clean up intro and credits",
      "Clean up subtitles",
      "Clean up downloads",
      "Clean up metadata cache",
      "Clean up activity",
      "Back up database",
      "Optimize database",
    ]);
  });

  it("says a failed run failed, with the reason on hover", async () => {
    serveMockApi();
    renderApp("/admin/tasks");
    await screen.findByRole("heading", { name: "Maintenance" });
    const backup = rowOf(section("Maintenance"), "Back up database");
    expect(cells(backup)[1]).toBe("Failed 8 hours ago");
    expect(within(backup).getByText("8 hours ago")).toHaveAttribute(
      "title",
      expect.stringContaining("The backup folder is full."),
    );
  });

  it("shows a muted — as the next run of a task every library is set to never for", async () => {
    const api = serveMockApi();
    delete taskOf(api.tasks, "find-markers").nextRunAt;
    renderApp("/admin/tasks");
    await screen.findByRole("heading", { name: "Library" });
    expect(cells(rowOf(section("Library"), "Intro and credits"))[2]).toBe("—");
  });

  it("shows Activity rows only while they have work, and hides Activity when none does", async () => {
    const api = serveMockApi();
    Object.assign(taskOf(api.tasks, "library-changes"), { progress: { done: 0, total: 0 }, running: true });
    renderApp("/admin/tasks");
    await screen.findByRole("heading", { name: "Activity" });
    const activity = section("Activity");
    expect(within(activity).queryByRole("columnheader")).not.toBeInTheDocument();
    expect(rows(activity).map((r) => cells(r)[0])).toEqual(["Downloads", "Library changes"]);
    // Work that was asked for has no count. It shows only what it's
    // doing, with no bar.
    expect(cells(rowOf(activity, "Downloads"))).toEqual(["Downloads", "", "", "Preparing", ""]);
    expect(within(rowOf(activity, "Downloads")).queryByRole("progressbar")).not.toBeInTheDocument();
    // Library changes finishes what it starts, so it has neither Stop nor
    // Run now.
    expect(cells(rowOf(activity, "Library changes"))).toEqual(["Library changes", "", "", "Removing", ""]);

    for (const id of ["prepare-downloads", "library-changes"]) {
      const task = taskOf(api.tasks, id);
      task.running = false;
      delete task.progress;
    }
    sendEvent({ type: "event", kind: "tasksChanged" });
    await waitFor(() => expect(screen.queryByRole("heading", { name: "Activity" })).not.toBeInTheDocument(), {
      timeout: 3000,
    });
  });

  it("runs a task now", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/tasks");
    await user.click(await screen.findByRole("button", { name: "Run Back up database now" }));
    await waitFor(() =>
      expect(sent("POST", "/tasks/{task}/run")[0]?.url.pathname).toBe("/api/v1/tasks/make-backup/run"),
    );
  });

  it("tells of a run started from Run now when it ends, in red when it failed", async () => {
    const api = serveMockApi();
    let outcome: { summary: string; error?: string } = { summary: "Optimized" };
    onRequest("post", "/tasks/{task}/run", (_, params) => {
      taskOf(api.tasks, params.task ?? "").lastRun = { finishedAt: new Date().toISOString(), ...outcome };
    });
    const { user } = renderApp("/admin/tasks");
    await user.click(await screen.findByRole("button", { name: "Run Optimize database now" }));
    const done = await screen.findByText("Optimize database finished", {}, { timeout: 3000 });
    expect(done.closest("[data-part=toast]")).toHaveAttribute("data-tone", "success");
    expect(within(done.closest("[data-part=toast]") as HTMLElement).getByText("Optimized")).toBeInTheDocument();

    outcome = { summary: "", error: "disk I/O error" };
    await user.click(screen.getByRole("button", { name: "Run Clean up downloads now" }));
    const failed = await screen.findByText("Clean up downloads failed", {}, { timeout: 3000 });
    expect(failed.closest("[data-part=toast]")).toHaveAttribute("data-tone", "danger");
  });

  it("stops a task's work after asking", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/tasks");
    await user.click(await screen.findByRole("button", { name: "Stop Intro and credits" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Stop Intro and credits?" });
    await user.click(within(confirm).getByRole("button", { name: "Stop" }));
    await waitFor(() =>
      expect(sent("POST", "/tasks/{task}/stop")[0]?.url.pathname).toBe("/api/v1/tasks/find-markers/stop"),
    );
    await waitFor(() => expect(screen.getByRole("button", { name: "Run Intro and credits now" })).toBeInTheDocument());
  });
});

describe("A task's page", () => {
  it("shows its progress with Stop while it runs, then its failed items, each with Retry, and Retry all", async () => {
    const api = serveMockApi();
    Object.assign(taskOf(api.tasks, "make-previews"), {
      progress: { done: 1, total: 6 },
      left: 0,
      running: true,
      stoppable: true,
    });
    const { user } = renderApp("/admin/tasks/make-previews");
    expect(await screen.findByRole("heading", { name: "Preview thumbnails" })).toBeInTheDocument();
    expect(
      within(screen.getByRole("region", { name: "Preview thumbnails" })).getByRole("link", { name: "Tasks" }),
    ).toHaveAttribute("href", "/admin/tasks");
    expect(document.querySelector('[data-part="task-status"]')).toHaveTextContent("Generating1 of 6");
    expect(screen.getByRole("button", { name: "Stop Preview thumbnails" })).toBeInTheDocument();

    const failed = await waitFor(() => section("Failed"));
    const row = rowOf(failed, "Heat (1995)/Heat.mkv");
    expect(row).toHaveTextContent(/ffmpeg couldn't decode the video: the file is truncated\. · .* ago/);
    expect(within(failed).getAllByRole("listitem")).toHaveLength(2);
    await user.click(within(row).getByRole("button", { name: "Retry Heat (1995)/Heat.mkv" }));
    const id = api.taskFailures["make-previews"]?.[0]?.itemId;
    await waitFor(() => expect(sent("POST", "/tasks/{task}/retry")[0]?.body).toEqual({ itemIds: [id] }));

    onRequest("post", "/tasks/{task}/retry", () => {
      api.taskFailures["make-previews"] = [];
    });
    await user.click(within(failed).getByRole("button", { name: "Retry all" }));
    await waitFor(() => expect(sent("POST", "/tasks/{task}/retry")).toHaveLength(2));
    expect(sent("POST", "/tasks/{task}/retry")[1]?.url.pathname).toBe("/api/v1/tasks/make-previews/retry");
    expect(sent("POST", "/tasks/{task}/retry")[1]?.body).toBeUndefined();
    expect(await screen.findByText("Nothing failed")).toBeInTheDocument();
  });

  it("says how much is left while it doesn't run, with Run now", async () => {
    serveMockApi();
    renderApp("/admin/tasks/make-previews");
    await screen.findByRole("heading", { name: "Preview thumbnails" });
    expect(document.querySelector('[data-part="task-status"]')).toHaveTextContent("18,489 left");
    expect(screen.getByRole("button", { name: "Run Preview thumbnails now" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Stop Preview thumbnails" })).not.toBeInTheDocument();
  });

  it("names a scan's step, and has no Retry all for one failed item", async () => {
    serveMockApi();
    renderApp("/admin/tasks/scan-libraries");
    expect(await screen.findByRole("heading", { name: "Scan all libraries" })).toBeInTheDocument();
    expect(document.querySelector('[data-part="task-status"]')).toHaveTextContent("Reading files37 of 120");
    const failed = await waitFor(() => section("Failed"));
    expect(rowOf(failed, "Broken Reel (2001)/Broken Reel.mkv")).toHaveTextContent(
      /The file is truncated: ffprobe found no video stream\. · .* ago/,
    );
    expect(within(failed).queryByRole("button", { name: "Retry all" })).not.toBeInTheDocument();
  });

  it("doesn't exist for a task that keeps no failures", async () => {
    serveMockApi();
    renderApp("/admin/tasks/make-backup");
    expect(await screen.findByRole("heading", { name: "This page doesn't exist." })).toBeInTheDocument();
  });
});
