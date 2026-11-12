import { waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { WorkDetail } from "@/api/schemas";
import { locationOf, renderApp, screen } from "@/test/render";
import { http, mockId, mockIds, scenarios, sent, serveMockApi, server } from "@/test/server";

/** Dune: Part Two's versions, 4K HDR first. */
const dune4K = mockId(110);
const dune1080 = mockId(111);

/** The ID of the image the hero shows. */
function heroImageId(): string | undefined {
  const src = document.querySelector('[data-part="hero"] img:not([aria-hidden])')?.getAttribute("src") ?? "";
  return /\/images\/([^/?]+)/.exec(src)?.[1];
}

/** Holds requests for one work's detail until released. */
function holdDetail(id: string): () => void {
  let release = () => {};
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  server.use(
    http.get("/api/v1/works/{id}", async ({ params }) => {
      if (params.id === id) await held;
    }),
  );
  return release;
}

describe("A work's page while its images load", () => {
  it("blurs the backdrop behind the hero, and shows its rows' cards in their colour", async () => {
    // A hash of its own, which no earlier page has drawn without a canvas.
    serveMockApi((api) => {
      const images = (api.works[mockIds.dunePartTwo] as WorkDetail).images;
      if (images.backdrop) images.backdrop.blurhash = "L6PZfSi_.AyE_3t7t7R**0o#DgR4";
    });
    const context = {
      createImageData: (w: number, h: number) => ({ data: new Uint8ClampedArray(w * h * 4) }),
      putImageData() {},
    };
    const draw = vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(context as never);
    const png = vi.spyOn(HTMLCanvasElement.prototype, "toDataURL").mockReturnValue("data:image/png;base64,blur");
    try {
      renderApp(`/works/${mockIds.dunePartTwo}`);
      const backdrop = await waitFor(() => {
        const el = document.querySelector('[data-part="hero-backdrop"] [data-part="artwork-placeholder"]');
        if (!el) throw new Error("no backdrop placeholder");
        return el;
      });
      expect(backdrop.tagName).toBe("IMG");
      expect(backdrop.getAttribute("src")).toBe("data:image/png;base64,blur");
      const trailers = await waitFor(() => {
        const el = document.querySelector('[data-part="row"] [data-part="artwork-placeholder"]');
        if (!el) throw new Error("no card placeholder");
        return el;
      });
      expect(trailers.tagName).toBe("DIV");
      expect(document.querySelectorAll('img[data-part="artwork-placeholder"]')).toHaveLength(1);
    } finally {
      draw.mockRestore();
      png.mockRestore();
    }
  });
});

describe("Loading a work's page", () => {
  it("shows the hero's shape, with the title and year of the card it was opened from", async () => {
    serveMockApi();
    const release = holdDetail(mockIds.dunePartTwo);
    const { user } = renderApp("/search?q=dune: part");
    await user.click((await screen.findAllByRole("link", { name: /^Dune: Part Two/ }))[0] as HTMLElement);
    const heading = await screen.findByRole("heading", { name: "Dune: Part Two", level: 1 });
    expect(heading.closest('[aria-busy="true"]')).not.toBeNull();
    expect(within(heading.parentElement as HTMLElement).getByText("2024")).toBeInTheDocument();
    release();
    expect(await screen.findByRole("link", { name: "Resume 1:12:03" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Dune: Part Two", level: 1 }).closest("[aria-busy]")).toBeNull();
  });

  it("shows the hero's shape alone when opened by URL", async () => {
    serveMockApi();
    const release = holdDetail(mockIds.dunePartTwo);
    renderApp(`/works/${mockIds.dunePartTwo}`);
    expect(await screen.findByText("Loading…")).toBeInTheDocument();
    expect(document.querySelector('[aria-busy="true"] [data-part="hero"]')).not.toBeNull();
    expect(screen.queryByRole("heading", { level: 1 })).not.toBeInTheDocument();
    release();
    expect(await screen.findByRole("heading", { name: "Dune: Part Two", level: 1 })).toBeInTheDocument();
  });
});

describe("Movie page", () => {
  it("shows one Trailers row, the folder's trailer first and then YouTube's, only for a movie that has any", async () => {
    serveMockApi();
    const { unmount } = renderApp(`/works/${mockIds.dunePartTwo}`);
    const row = (await screen.findByRole("heading", { name: "Trailers" })).closest("section");
    if (!row) throw new Error("no Trailers row");
    const cards = [...row.querySelectorAll<HTMLElement>('[data-part="card"]')];
    expect(cards.map((c) => c.dataset.kind)).toEqual(["extra", "trailer", "trailer"]);
    // The folder's trailer plays here.
    const [local, first, second] = cards as [HTMLElement, HTMLElement, HTMLElement];
    expect(within(local).getByText("Official Trailer")).toBeInTheDocument();
    expect(within(local).getByText("2:00")).toBeInTheDocument();
    expect(within(local).queryByText("In your library")).not.toBeInTheDocument();
    expect(local.querySelector("a")?.getAttribute("href")).toMatch(/^\/works\//);
    // YouTube's open there, in a new tab.
    expect(first).toHaveAttribute("href", "https://www.youtube.com/watch?v=Way9Dexny3w");
    expect(first).toHaveAttribute("target", "_blank");
    expect(within(first).getByText("Official Trailer 3")).toBeInTheDocument();
    expect(within(first).getByText("YouTube ↗")).toBeInTheDocument();
    expect(first).toHaveAccessibleName("Official Trailer 3, Opens in YouTube");
    expect(within(first).queryByText("Opens in YouTube")).not.toBeInTheDocument();
    expect(first.querySelector("img")).not.toBeNull();
    // One whose thumbnail the server doesn't have shows the bare tile, its title under it alone.
    expect(second).toHaveAttribute("href", "https://www.youtube.com/watch?v=_YUzQa_1RCE");
    expect(second.querySelector("img")).toBeNull();
    expect(within(second).getAllByText("Official Trailer 2")).toHaveLength(1);
    // No button and no menu item leads to a trailer.
    expect(screen.queryByRole("link", { name: "Trailer" })).not.toBeInTheDocument();
    // The other extras keep their own rows.
    expect(screen.getByRole("heading", { name: "Featurettes" })).toBeInTheDocument();
    unmount();
    renderApp(`/works/${mockIds.dune}`);
    expect(await screen.findByRole("heading", { name: "Dune" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Trailers" })).not.toBeInTheDocument();
  });

  it("offers Resume with the time, Play from beginning beside it, the time left and end under it, and the pickers", async () => {
    serveMockApi();
    const { router, user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    expect(await screen.findByRole("heading", { name: "Dune: Part Two" })).toBeInTheDocument();
    expect(screen.getByText("2024 · 2h 46m · ★ 8.2 · PG-13 · Science Fiction, Adventure")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Resume 1:12:03" })).toHaveAttribute(
      "href",
      `/watch/${mockIds.dunePartTwo}?version=${dune4K}`,
    );
    expect(screen.getByText(/^1 hr 34 min left · Ends at \d{1,2}:\d\d/)).toBeInTheDocument();
    await waitFor(() => expect(document.title).toBe("Dune: Part Two – Kivo"));
    expect(screen.queryByRole("link", { name: /from the start/ })).not.toBeInTheDocument();
    // Nothing played yet, so the first version is selected, and tracks
    // default to Automatic, labelled with what plays.
    expect(screen.getByRole("button", { name: /Version/ })).toHaveTextContent(/^4K/);
    expect(screen.getByRole("button", { name: /Automatic \(English · 7\.1 · TrueHD\)/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Automatic \(Off\)/ })).toBeInTheDocument();
    expect(screen.queryByText("Remembered for this show")).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Cast & Crew" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Trailers" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "Part of Dune Collection" })).toBeInTheDocument();

    // Play from beginning is the button beside Resume, not in ⋯.
    await user.click(screen.getByRole("button", { name: "More actions for Dune: Part Two" }));
    const menu = await screen.findByRole("menu");
    expect(within(menu).queryByRole("menuitem", { name: "Play from beginning" })).not.toBeInTheDocument();
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .slice(0, 2)
        .map((i) => i.textContent),
    ).toEqual(["Add to List", "Go to library"]);
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "Play from beginning" }));
    await waitFor(() => expect(router.state.location.pathname).toBe(`/watch/${mockIds.dunePartTwo}`));
    expect(router.state.location.search).toEqual({ version: dune4K, from: "start" });
  });

  it("keeps track choices in the URL and carries them into playback", async () => {
    serveMockApi();
    const { router, user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await user.click(await screen.findByRole("button", { name: /Automatic \(Off\)/ }));
    await user.click(await screen.findByRole("option", { name: /^English · SDH/ }));
    await waitFor(() => expect(router.state.location.search).toEqual({ subtitle: "4k-sub-en-sdh" }));
    expect(screen.getByRole("link", { name: "Resume 1:12:03" })).toHaveAttribute(
      "href",
      `/watch/${mockIds.dunePartTwo}?version=${dune4K}&subtitle=4k-sub-en-sdh`,
    );
    await user.click(screen.getByRole("button", { name: /English · SDH/ }));
    await user.click(await screen.findByRole("option", { name: "Automatic (Off)" }));
    await waitFor(() => expect(router.state.location.search).toEqual({}));

    // Track IDs belong to one file, so changing version resets them. The
    // 1080p version has one audio track and no subtitles, so there's
    // nothing to choose but Search subtitles.
    await user.click(screen.getByRole("button", { name: /Version/ }));
    await user.click(await screen.findByRole("option", { name: /^1080p/ }));
    await waitFor(() => expect(router.state.location.search).toEqual({ version: expect.any(String) }));
    expect(screen.queryByRole("button", { name: /Audio/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Automatic \(Off\)/ }));
    expect((await screen.findAllByRole("option")).map((o) => o.textContent)).toEqual([
      "Automatic (Off)",
      "Off",
      "Search subtitles…",
    ]);
  });

  it("searches subtitles from the picker, and chooses the one it downloads for the play", async () => {
    const api = serveMockApi(scenarios.viewer);
    const { router, user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await user.click(await screen.findByRole("button", { name: /Automatic \(Off\)/ }));
    await user.click(await screen.findByRole("option", { name: "Search subtitles…" }));
    const dialog = await screen.findByRole("dialog", { name: "Search subtitles" });
    expect(await within(dialog).findByText("English · Exact match")).toBeInTheDocument();
    await user.click(within(dialog).getAllByRole("button", { name: "Download" })[0] as HTMLElement);
    await waitFor(() => expect(router.state.location.search).toEqual({ subtitle: api.addedSubtitle.id }));
    expect(sent("POST", "/files/{id}/subtitles")[0]?.body).toEqual({ resultId: "os-1" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("plays the version selected, with the track chosen from it", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await user.click(await screen.findByRole("button", { name: /Automatic \(English/ }));
    await user.click(await screen.findByRole("option", { name: /^French/ }));
    await user.click(await screen.findByRole("link", { name: "Resume 1:12:03" }));
    await waitFor(() => expect(sent("POST", "/plays")).toHaveLength(1));
    expect(sent("POST", "/plays")[0]?.body).toMatchObject({ versionId: dune4K, audioTrackId: "4k-audio-fr" });
  });

  it("selects the version last played, with no Auto entry", async () => {
    serveMockApi((api) => {
      const work = api.works[mockIds.dunePartTwo] as WorkDetail;
      work.userState = { ...work.userState, played: false, versionId: dune1080 };
    });
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    expect(await screen.findByRole("link", { name: "Resume 1:12:03" })).toHaveAttribute(
      "href",
      `/watch/${mockIds.dunePartTwo}?version=${dune1080}`,
    );
    expect(screen.getByRole("button", { name: /Version/ })).toHaveTextContent(/^1080p/);
    await user.click(screen.getByRole("button", { name: /Version/ }));
    expect((await screen.findAllByRole("option")).map((o) => o.textContent)).toEqual([
      expect.stringMatching(/^4K/),
      expect.stringMatching(/^1080p/),
    ]);
  });

  it("names each version with its edition, a cut's versions together", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockIds.bladeRunner}`);
    await user.click(await screen.findByRole("button", { name: /Version/ }));
    const options = (await screen.findAllByRole("option")).map((o) => o.textContent);
    expect(options).toEqual([
      expect.stringMatching(/^1080p/),
      expect.stringMatching(/^Final Cut · 4K HDR/),
      expect.stringMatching(/^Final Cut · 1080p/),
    ]);
  });

  it("starts a version of another edition from the beginning", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockIds.bladeRunner}`);
    expect(await screen.findByRole("link", { name: /^Resume/ })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Version/ }));
    await user.click(await screen.findByRole("option", { name: /^Final Cut · 4K HDR/ }));
    expect(await screen.findByRole("link", { name: "Play" })).toBeInTheDocument();
    // Started from the beginning, it has no time left to show or start over from, only its end.
    expect(screen.queryByRole("button", { name: "Play from beginning" })).not.toBeInTheDocument();
    expect(screen.getByText(/^Ends at \d{1,2}:\d\d/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Version/ }));
    await user.click(await screen.findByRole("option", { name: /^1080p/ }));
    expect(await screen.findByRole("link", { name: /^Resume/ })).toBeInTheDocument();
  });

  it("heads a movie its folder labels with its title and edition", async () => {
    serveMockApi();
    renderApp(`/works/${mockIds.bladeRunnerFinalCut}`);
    expect(await screen.findByRole("heading", { level: 1, name: "Blade Runner · Final Cut" })).toBeInTheDocument();
  });

  it("says the file is missing in place of Play", async () => {
    serveMockApi();
    renderApp(`/works/${mockIds.missingReel}`);
    expect(await screen.findByText("This file is missing")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Play" })).not.toBeInTheDocument();
  });

  it("toggles + Watchlist on and off through the API", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockIds.dunePartTwo}`);
    await user.click(await screen.findByRole("button", { name: "Add to Watchlist" }));
    expect(await screen.findByRole("button", { name: "Remove from Watchlist" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(sent("POST", `/lists/${mockIds.myList}/works`)[0]?.body).toEqual({ workId: mockIds.dunePartTwo });
    await user.click(screen.getByRole("button", { name: "Remove from Watchlist" }));
    expect(await screen.findByRole("button", { name: "Add to Watchlist" })).toHaveAttribute("aria-pressed", "false");
    await waitFor(() =>
      expect(sent("DELETE", `/lists/${mockIds.myList}/works/${mockIds.dunePartTwo}`)).toHaveLength(1),
    );
  });
});

describe("Collection page", () => {
  it("shows its root works, with no Watchlist or watched toggles of its own", async () => {
    serveMockApi();
    renderApp(`/collections/${mockIds.duneCollection}`);
    expect(await screen.findByRole("heading", { name: "Dune Collection", level: 1 })).toBeInTheDocument();
    expect(screen.getByText("2 items")).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: /^Dune: Part Two/ })).toHaveAttribute(
      "href",
      `/works/${mockIds.dunePartTwo}`,
    );
    expect(screen.queryByRole("button", { name: "Add to Watchlist" })).not.toBeInTheDocument();
  });

  it("shows its poster, and its backdrop behind the hero", async () => {
    serveMockApi();
    renderApp(`/collections/${mockIds.duneCollection}`);
    const hero = (await screen.findByRole("heading", { name: "Dune Collection", level: 1 })).closest(
      "[data-part='hero']",
    );
    expect(hero?.querySelectorAll("img:not([aria-hidden])")).toHaveLength(2);
    expect(within(hero as HTMLElement).getByRole("img", { name: "Dune Collection" })).toBeInTheDocument();
  });
});

describe("Series page", () => {
  it("plays Next Up and opens on its season", async () => {
    serveMockApi();
    renderApp(`/works/${mockIds.expanse}`);
    expect(await screen.findByRole("link", { name: "Resume 30:00" })).toHaveAttribute("href", `/watch/${mockId(1303)}`);
    expect(screen.getByText("S3 · E3 · Episode 3 · 14 min left")).toBeInTheDocument();
    expect(screen.getByText("2015 to 2022 · 4 seasons · ★ 8.5 · TV-14")).toBeInTheDocument();
    const strip = await screen.findByRole("navigation", { name: "Seasons" });
    expect(within(strip).getByRole("button", { name: /S3/ })).toHaveAttribute("aria-current", "true");
    // Specials come last.
    expect(
      within(strip)
        .getAllByRole("button")
        .map((b) => b.textContent),
    ).toEqual(["S1", "S2", "S38", "Specials2"]);
    expect(await screen.findByRole("link", { name: /E4 Reload/ })).toBeInTheDocument();
  });

  it("shows the series' trailers on YouTube in a Trailers row", async () => {
    serveMockApi();
    renderApp(`/works/${mockIds.expanse}`);
    const row = (await screen.findByRole("heading", { name: "Trailers" })).closest("section");
    if (!row) throw new Error("no Trailers row");
    const card = within(row).getByRole("link", { name: "Official Trailer, Opens in YouTube" });
    expect(card).toHaveAttribute("href", "https://www.youtube.com/watch?v=kQuTJdE3j2Q");
    expect(card).toHaveAttribute("target", "_blank");
  });

  it("asks once before marking a season's episodes played", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockIds.expanse}`);
    await user.click(await screen.findByRole("button", { name: "Mark season as watched" }));
    const dialog = await screen.findByRole("alertdialog");
    expect(within(dialog).getByRole("heading")).toHaveTextContent("Mark 8 episodes as watched?");
    await user.click(within(dialog).getByRole("button", { name: "Mark as watched" }));
    await waitFor(() =>
      expect(sent("PATCH", `/works/${mockIds.expanseSeason3}/user-state`).map((r) => r.body)).toEqual([
        { played: true },
      ]),
    );
  });

  it("unchecks a watched season's check", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockIds.expanse}`);
    await user.click(await screen.findByRole("button", { name: "Mark season as watched" }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Mark as watched" }));
    await user.click(await screen.findByRole("button", { name: "Mark season as unwatched", pressed: true }));
    expect(await screen.findByRole("button", { name: "Mark season as watched", pressed: false })).toBeInTheDocument();
    expect(sent("PATCH", `/works/${mockIds.expanseSeason3}/user-state`).map((r) => r.body)).toEqual([
      { played: true },
      { played: false },
    ]);
  });

  it("opens a season's own URL on the series page at that season", async () => {
    serveMockApi();
    const { router } = renderApp(`/works/${mockIds.expanseSeason3}`);
    await screen.findByRole("navigation", { name: "Seasons" });
    expect(locationOf(router)).toBe(`/works/${mockIds.expanse}?season=${mockIds.expanseSeason3}`);
  });
});

describe("Episode list", () => {
  it("plays an episode straight from its still, resuming one in progress", async () => {
    serveMockApi();
    const { router, user } = renderApp(`/works/${mockIds.expanse}`);
    expect(await screen.findByRole("link", { name: "Resume S3 E3: Episode 3" })).toHaveAttribute(
      "href",
      `/watch/${mockId(1303)}`,
    );
    await user.click(screen.getByRole("link", { name: "Play S3 E4: Reload" }));
    await waitFor(() => expect(locationOf(router)).toBe(`/watch/${mockIds.expanseReload}`));
  });

  it("opens the episode page from the row", async () => {
    serveMockApi();
    const { router, user } = renderApp(`/works/${mockIds.expanse}`);
    await user.click(await screen.findByRole("link", { name: /E4 Reload/ }));
    await waitFor(() => expect(locationOf(router)).toBe(`/works/${mockIds.expanseReload}`));
    expect(await screen.findByRole("heading", { name: "S3 · E4 · Reload", level: 1 })).toBeInTheDocument();
  });

  it("holds a row for each of the season's episodes until they load", async () => {
    serveMockApi();
    // The open season's episodes wait until released.
    let release = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.get("/api/v1/works/{id}/children", async ({ params }) => {
        if (params.id === mockIds.expanseSeason3) await held;
      }),
    );
    renderApp(`/works/${mockIds.expanse}`);
    await screen.findByRole("navigation", { name: "Seasons" });
    const list = document.querySelector('[data-part="episode-list"]');
    expect(list).toHaveAttribute("aria-busy", "true");
    expect(list?.querySelectorAll('li[aria-hidden="true"]')).toHaveLength(10);
    release();
    expect(await screen.findByRole("link", { name: /E4 Reload/ })).toBeInTheDocument();
    expect(document.querySelectorAll('[data-part="episode-list"] li[aria-hidden="true"]')).toHaveLength(0);
  });

  it("lists a single-season series' episodes without a season strip", async () => {
    serveMockApi();
    renderApp(`/works/${mockIds.severance}`);
    expect(await screen.findByRole("link", { name: "Play S1 E2: Half Loop" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Episodes" })).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Seasons" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Mark season as watched" })).not.toBeInTheDocument();
  });
});

describe("Episode page", () => {
  it("shows the still, the series link, the episode's details and its season", async () => {
    const api = serveMockApi();
    renderApp(`/works/${mockIds.expanseReload}`);
    expect(await screen.findByRole("heading", { name: "S3 · E4 · Reload", level: 1 })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "The Expanse" })).toHaveAttribute(
      "href",
      `/works/${mockIds.expanse}?season=${mockIds.expanseSeason3}`,
    );
    expect(screen.getByText("Jan 28, 2017 · 44m · ★ 8.4 · TV-14")).toBeInTheDocument();
    expect(screen.getByText("Holden races to stop an asteroid before it reaches Earth.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Play" })).toHaveAttribute(
      "href",
      `/watch/${mockIds.expanseReload}?version=${api.works[mockIds.expanseReload]?.versions[0]?.id}`,
    );
    expect(heroImageId()).toBe(api.works[mockIds.expanseReload]?.images.still?.id);
    // The rest of the season, with the current episode marked and no
    // season switcher.
    expect(screen.getByRole("heading", { name: "Season 3", level: 2 })).toBeInTheDocument();
    expect(await screen.findByRole("link", { current: "page" })).toHaveTextContent("E4 Reload");
    expect(screen.getByRole("link", { name: "Play S3 E5: Episode 5" })).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Seasons" })).not.toBeInTheDocument();
  });

  it("says a track chosen is remembered for the show", async () => {
    const api = serveMockApi();
    api.works[mockIds.expanseReload]?.versions[0]?.tracks.push({
      id: "reload-sub-en",
      type: "subtitle",
      codec: "subrip",
      language: "en",
      default: false,
      name: "English",
      fullName: "English · SRT · External",
      subtitle: { source: "external" },
    });
    renderApp(`/works/${mockIds.expanseReload}`);
    expect(await screen.findByRole("button", { name: /Automatic \(Off\)/ })).toBeInTheDocument();
    expect(screen.getByText("Remembered for this show")).toBeInTheDocument();
    // One audio track, so there's nothing to choose.
    expect(screen.queryByRole("button", { name: /Audio/ })).not.toBeInTheDocument();
  });

  it("falls back to the series backdrop without a still", async () => {
    const api = serveMockApi();
    renderApp(`/works/${mockIds.severancePilot}`);
    expect(
      await screen.findByRole("heading", { name: "S1 · E1 · Good News About Hell", level: 1 }),
    ).toBeInTheDocument();
    const backdrop = api.works[mockIds.severance]?.images.backdrop?.id;
    await waitFor(() => expect(heroImageId()).toBe(backdrop));
    expect(screen.queryByRole("link", { name: "Previous episode" })).not.toBeInTheDocument();
  });

  it("steps to the previous and next episodes, across seasons", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${mockId(1206)}`);
    expect(await screen.findByRole("heading", { name: "S2 · E6 · Episode 6", level: 1 })).toBeInTheDocument();
    await user.click(screen.getByRole("link", { name: "Next episode" }));
    expect(await screen.findByRole("heading", { name: "S3 · E1 · Episode 1", level: 1 })).toBeInTheDocument();
    await user.click(screen.getByRole("link", { name: "Previous episode" }));
    expect(await screen.findByRole("heading", { name: "S2 · E6 · Episode 6", level: 1 })).toBeInTheDocument();
  });
});
