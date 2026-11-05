import { onlineManager } from "@tanstack/react-query";
import { waitFor } from "@testing-library/react";
import { HttpResponse, http as untyped, ws } from "msw";
import { afterEach, describe, expect, it } from "vitest";
import { renderApp, screen } from "@/test/render";
import { closeEvents, sent, serveMockApi, server } from "@/test/server";

/** Home with its event socket open, which the server then drops, and stops answering if `down`. */
async function socketDrops(down = false) {
  serveMockApi();
  renderApp("/");
  await screen.findByRole("heading", { name: "Continue Watching" });
  if (down) server.use(untyped.get("*/api/v1/server", () => HttpResponse.error()));
  const asked = sent("GET", "/server").length;
  await waitFor(() => expect(closeEvents(1011)).toBeGreaterThan(0));
  return asked;
}

/** Answers to /server held until the test lets them go, each as it says. */
function holdServerAnswers() {
  const waiting: ((answer: Response) => void)[] = [];
  server.use(untyped.get("*/api/v1/server", () => new Promise<Response>((resolve) => waiting.push(resolve))));
  return {
    asked: () => waiting.length,
    answer: (i: number, response: Response) => waiting[i]?.(response),
  };
}

/** Each socket from now on is dropped as it opens, counting the attempts. */
function refuseSockets() {
  const attempts = { count: 0 };
  server.use(
    ws.link("*/api/v1/events").addEventListener("connection", (event) => {
      event.stopImmediatePropagation();
      attempts.count++;
      event.client.close(1011);
    }),
  );
  return attempts;
}

describe("The connection banner", () => {
  afterEach(() => onlineManager.setOnline(true));

  it("stays away while the server answers, when only the event socket fails", async () => {
    const asked = await socketDrops();
    // It asks whether the server answers before saying it can't reach it.
    await waitFor(() => expect(sent("GET", "/server").length).toBe(asked + 1));
    expect(screen.queryByText(/^Can't reach/)).not.toBeInTheDocument();
  });

  it("says the server can't be reached when it doesn't answer, with Retry now", async () => {
    await socketDrops(true);
    expect(await screen.findByText("Can't reach Living room. Reconnecting…")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry now" })).toBeInTheDocument();
  });

  it("asks the server even when the browser says it's offline", async () => {
    serveMockApi();
    renderApp("/");
    await screen.findByRole("heading", { name: "Continue Watching" });
    server.use(untyped.get("*/api/v1/server", () => HttpResponse.error()));
    window.dispatchEvent(new Event("offline"));
    await waitFor(() => expect(closeEvents(1011)).toBeGreaterThan(0));
    expect(await screen.findByText(/^Can't reach Living room\./)).toBeInTheDocument();
  });

  it("takes a slow answer that comes after the next attempt failed", async () => {
    serveMockApi();
    renderApp("/");
    await screen.findByRole("heading", { name: "Continue Watching" });
    const answers = holdServerAnswers();
    const attempts = refuseSockets();
    await waitFor(() => expect(closeEvents(1011)).toBeGreaterThan(0));
    await waitFor(() => expect(answers.asked()).toBe(1));
    // The next attempt fails while the first question is still out.
    await waitFor(() => expect(attempts.count).toBe(1), { timeout: 2000 });
    await new Promise((resolve) => setTimeout(resolve, 100));
    answers.answer(0, HttpResponse.error());
    expect(await screen.findByText(/^Can't reach Living room\./)).toBeInTheDocument();
  });

  it("forgets an earlier outage once the socket opens, when a later one is the socket's alone", async () => {
    await socketDrops(true);
    await screen.findByText(/^Can't reach Living room\./);
    const answers = holdServerAnswers();
    // The socket comes back on its next attempt.
    await waitFor(() => expect(screen.queryByText(/^Can't reach/)).not.toBeInTheDocument(), { timeout: 2000 });
    await waitFor(() => expect(closeEvents(1011)).toBeGreaterThan(0));
    await waitFor(() => expect(answers.asked()).toBe(1));
    expect(screen.queryByText(/^Can't reach/)).not.toBeInTheDocument();
    answers.answer(0, HttpResponse.json({}));
    await new Promise((resolve) => setTimeout(resolve, 100));
    expect(screen.queryByText(/^Can't reach/)).not.toBeInTheDocument();
  });
});
