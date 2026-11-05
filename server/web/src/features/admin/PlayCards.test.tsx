import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderApp, screen } from "@/test/render";
import { onRequest, scenarios, sent, serveMockApi } from "@/test/server";

const card = (name: RegExp) => screen.findByRole("button", { name });

describe("Play cards", () => {
  it("shows each play as a card: what, who, where, the method and the stream", async () => {
    serveMockApi(scenarios.nowPlaying);
    renderApp("/admin");

    const episode = await card(/^The Expanse, anna/);
    expect(within(episode).getByText(/^The Expanse/)).toHaveTextContent("The Expanse · S3 · E4");
    expect(within(episode).getByText("anna · anna's iPhone · Local")).toBeInTheDocument();
    expect(within(episode).getByText("Direct Play")).toBeInTheDocument();
    expect(within(episode).getByText("H.264 High 1080p · 8.2 Mb/s")).toBeInTheDocument();

    const film = await card(/^Dune: Part Two, ben/);
    expect(within(film).getByText(/^Dune: Part Two/)).toHaveTextContent("Dune: Part Two · 2024");
    expect(within(film).getByText("ben · Firefox on Windows · Remote")).toBeInTheDocument();
    expect(within(film).getByText("Transcode (hw)")).toBeInTheDocument();
    // How far ahead the transcode is, and the main reason, shown on hover.
    expect(within(film).getByText("3:24 ahead")).toBeInTheDocument();
    expect(within(film).getByText(/^Transcode: /)).toBeInTheDocument();

    const book = await card(/^Leviathan Wakes, carla/);
    expect(within(book).getByText(/^Leviathan Wakes/)).toHaveTextContent("Leviathan Wakes · James S. A. Corey");
    expect(within(book).getByText("Paused")).toBeInTheDocument();
    expect(within(book).getByText("Unknown language · AAC 2.0 · Default")).toBeInTheDocument();
  });

  it("marks only a transcode on an accelerator (hw)", async () => {
    const api = serveMockApi(scenarios.nowPlaying);
    api.plays = api.plays.map((p) => (p.id === "now-2" ? { ...p, videoProcessing: "cpu" } : p));
    renderApp("/admin");

    const film = await card(/^Dune: Part Two, ben/);
    expect(within(film).getByText("Transcode")).toBeInTheDocument();
    expect(within(film).queryByText(/\(hw\)/)).toBeNull();
  });

  it("opens a transcode's details:who and where, the method, each track from source to sent with why, the transcoder and the bandwidth", async () => {
    serveMockApi(scenarios.nowPlaying);
    const { user } = renderApp("/admin");
    await user.click(await card(/^Dune: Part Two, ben/));

    const details = await screen.findByRole("dialog", { name: "Dune: Part Two" });
    const panel = within(details);
    expect(panel.getByText("2024")).toBeInTheDocument();
    expect(panel.getByText("ben · Firefox on Windows")).toBeInTheDocument();
    expect(panel.getByText("Remote · 203.0.113.24")).toBeInTheDocument();
    expect(panel.getByText("Firefox 132 · kivo-web index-3f9c2a")).toBeInTheDocument();
    expect(panel.getByText("1:12:03 / 2:46:00")).toBeInTheDocument();
    expect(panel.getByText(/^Started /)).toBeInTheDocument();
    expect(panel.getByText("Transcode (hw)")).toBeInTheDocument();

    const row = (name: string) => panel.getByRole("rowheader", { name }).closest("tr");
    expect(row("Video")).toHaveTextContent("VideoHEVC 4K HDR10H.264 1080pBurned-in subtitles");
    expect(row("Audio")).toHaveTextContent(
      "AudioTrueHD 7.1AAC 5.1Can't play TrueHDcodec: truehd, declared aac,ac3,eac3,opus,flac,mp3",
    );
    expect(row("Subtitles")).toHaveTextContent("SubtitlesFrench ASS · ForcedBurned in");

    const stat = (label: string) => panel.getByText(label).nextSibling;
    expect(stat("Transcoder")).toHaveTextContent("1.8× realtime3:24 ahead");
    expect(stat("Bandwidth")).toHaveTextContent("8.4 Mb/s42 Mb/s available");

    await user.click(panel.getByRole("button", { name: "Show diagnostics" }));
    expect(panel.getByText(/ffmpeg -hwaccel qsv/)).toBeInTheDocument();
  });

  it("opens a direct play's details: each track sent the same, without a reason, subtitles or the transcoder", async () => {
    serveMockApi(scenarios.nowPlaying);
    const { user } = renderApp("/admin");
    await user.click(await card(/^The Expanse, anna/));

    const details = await screen.findByRole("dialog", { name: "The Expanse" });
    const panel = within(details);
    expect(panel.getByText(/^S3 · E4 · /)).toBeInTheDocument();
    expect(panel.getByText("anna · anna's iPhone")).toBeInTheDocument();
    expect(panel.getByText("Local · 192.168.1.42")).toBeInTheDocument();
    expect(panel.getByText("iPhone · iPhone16,2 · OS 18.1 · kivo-apple 1.4.0")).toBeInTheDocument();
    expect(panel.getByText("Direct Play")).toBeInTheDocument();

    expect(panel.getAllByRole("rowheader").map((h) => h.textContent)).toEqual(["Video", "Audio"]);
    for (const h of panel.getAllByRole("rowheader")) expect(h.closest("tr")).toHaveTextContent(/Same$/);
    expect(panel.queryByText("Transcoder")).toBeNull();
    expect(panel.getByText("Bandwidth").nextSibling).toHaveTextContent(/^8\.2 Mb\/s$/);
    expect(panel.queryByRole("button", { name: "Show diagnostics" })).toBeNull();
  });

  it("stops a play with a message to the viewer, sending no position: its owner's last report stands", async () => {
    const api = serveMockApi(scenarios.nowPlaying);
    onRequest("post", "/plays/{id}/stop", () => {
      api.plays = api.plays.filter((s) => s.id !== "now-2");
    });
    const { user } = renderApp("/admin");
    await user.click(await card(/^Dune: Part Two, ben/));
    const details = await screen.findByRole("dialog", { name: "Dune: Part Two" });
    const panel = within(details);
    expect(panel.queryByLabelText(/Message to the viewer/)).toBeNull();

    // Stop stream… asks first, and Cancel closes it without sending.
    await user.click(panel.getByRole("button", { name: "Stop stream…" }));
    const cancel = within(await screen.findByRole("dialog", { name: "Stop Dune: Part Two for ben?" }));
    await user.click(cancel.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: /^Stop / })).toBeNull());
    expect(sent("POST", "/plays/now-2/stop")).toHaveLength(0);

    await user.click(panel.getByRole("button", { name: "Stop stream…" }));
    const ask = within(await screen.findByRole("dialog", { name: "Stop Dune: Part Two for ben?" }));
    await user.type(ask.getByLabelText(/Message to the viewer/), "Server maintenance");
    await user.click(ask.getByRole("button", { name: "Stop stream" }));

    await waitFor(() => expect(sent("POST", "/plays/now-2/stop")).toHaveLength(1));
    expect(sent("POST", "/plays/now-2/stop")[0]?.body).toEqual({ message: "Server maintenance" });
    await waitFor(() => expect(screen.queryByRole("button", { name: /^Dune: Part Two, ben/ })).toBeNull());
    expect(screen.queryByRole("dialog", { name: "Dune: Part Two" })).toBeNull();
    expect(screen.getByRole("button", { name: /^The Expanse, anna/ })).toBeInTheDocument();
  });

  it("stops a play from its card, after asking, with the same optional message", async () => {
    serveMockApi(scenarios.nowPlaying);
    const { user } = renderApp("/admin");
    await user.click(await screen.findByRole("button", { name: "Stop Dune: Part Two for ben" }));
    const ask = within(await screen.findByRole("dialog", { name: "Stop Dune: Part Two for ben?" }));
    expect(ask.getByLabelText(/Message to the viewer/)).toHaveValue("");
    await user.click(ask.getByRole("button", { name: "Stop stream" }));
    await waitFor(() => expect(sent("POST", "/plays/now-2/stop")[0]?.body).toEqual({}));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: /^Stop / })).toBeNull());
  });
});
