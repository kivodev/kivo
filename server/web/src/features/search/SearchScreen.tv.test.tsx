import { fireEvent, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { signedOut } from "@/api/session";
import { renderApp, screen } from "@/test/render";
import { serveMockApi } from "@/test/server";
import { focusTv } from "@/tv/spatial";
import { stopTv } from "@/tv/start";
import { forgetLastSearch } from "./SearchScreen";

const { keyboardProps } = vi.hoisted(() => ({ keyboardProps: vi.fn() }));
vi.mock("@/tv/keyboard/GridKeyboard", async (original) => {
  const { GridKeyboard } = await original<typeof import("@/tv/keyboard/GridKeyboard")>();
  return {
    GridKeyboard: (props: Parameters<typeof GridKeyboard>[0]) => {
      keyboardProps(props);
      return <GridKeyboard {...props} />;
    },
  };
});

const focused = () => document.activeElement?.getAttribute("data-tv-focus-key");
const press = (key: string) => {
  fireEvent.keyDown(document.activeElement ?? window, { key });
  fireEvent.keyUp(document.activeElement ?? window, { key });
};

// Moves the remote's focus to a key and presses OK on it.
async function ok(focusKey: string) {
  focusTv(focusKey);
  await waitFor(() => expect(focused()).toBe(focusKey));
  press("Enter");
}

describe("Search on TV", () => {
  beforeEach(() => {
    forgetLastSearch();
  });
  afterEach(() => {
    stopTv();
  });

  it("types on the grid keyboard, deletes, and moves Right from the keyboard to the results", {
    timeout: 10_000,
  }, async () => {
    serveMockApi();
    const { router } = renderApp("/search");
    // The keyboard has the screen's initial focus.
    await waitFor(() => expect(focused()).toBe("search:key:letters:a"));
    const field = screen.getByRole("searchbox", { name: "Search" });
    const before = keyboardProps.mock.lastCall?.[0];
    for (const c of "dunx") await ok(`search:key:letters:${c}`);
    const after = keyboardProps.mock.lastCall?.[0];
    for (const callback of ["onType", "onBackspace", "onClear", "onRight"]) {
      expect(after[callback]).toBe(before[callback]);
    }
    expect(field).toHaveValue("dunx");
    await ok("search:backspace");
    await ok("search:key:letters:e");
    expect(field).toHaveValue("dune");

    // The search follows the text after a pause, as on the web.
    await waitFor(() => expect(router.state.location.search).toEqual({ q: "dune" }));
    expect(await screen.findByRole("heading", { name: "Movies" })).toBeInTheDocument();

    // The last key of a row is at the keyboard's right edge.
    focusTv("search:key:letters:f");
    await waitFor(() => expect(focused()).toBe("search:key:letters:f"));
    press("ArrowRight");
    await waitFor(() => expect(focused()).toMatch(/^card:search:/));
  });

  it("says what to search for while nothing is typed, and the keyboard keeps the initial focus", async () => {
    serveMockApi();
    renderApp("/search");
    await waitFor(() => expect(focused()).toBe("search:key:letters:a"));
    expect(screen.getByText("Search your libraries by title, person or collection")).toBeInTheDocument();
    for (const c of "dune") await ok(`search:key:letters:${c}`);
    await waitFor(() =>
      expect(screen.queryByText("Search your libraries by title, person or collection")).not.toBeInTheDocument(),
    );
  });

  it("clears the text with Clear, and spaces words", async () => {
    serveMockApi();
    renderApp("/search");
    await waitFor(() => expect(focused()).toBe("search:key:letters:a"));
    const field = screen.getByRole("searchbox", { name: "Search" });
    await ok("search:key:letters:a");
    await ok("search:space");
    await ok("search:key:letters:b");
    expect(field).toHaveValue("a b");
    await ok("search:clear");
    expect(field).toHaveValue("");
  });

  it("opens on the last search typed while the app runs", async () => {
    serveMockApi();
    const { router } = renderApp("/search");
    await waitFor(() => expect(focused()).toBe("search:key:letters:a"));
    for (const c of "dune") await ok(`search:key:letters:${c}`);
    await waitFor(() => expect(router.state.location.search).toEqual({ q: "dune" }));

    await router.navigate({ to: "/" });
    await router.navigate({ to: "/search" });
    expect(await screen.findByRole("searchbox", { name: "Search" })).toHaveValue("dune");
    await waitFor(() => expect(router.state.location.search).toEqual({ q: "dune" }));
  });

  it("forgets the last search when signed out, so the next person on a shared TV doesn't see it", async () => {
    serveMockApi();
    const { router, queryClient } = renderApp("/search");
    await waitFor(() => expect(focused()).toBe("search:key:letters:a"));
    for (const c of "dune") await ok(`search:key:letters:${c}`);
    await waitFor(() => expect(router.state.location.search).toEqual({ q: "dune" }));

    await signedOut(router, queryClient, false);
    await router.navigate({ to: "/search" });
    expect(await screen.findByRole("searchbox", { name: "Search" })).toHaveValue("");
  });
});
