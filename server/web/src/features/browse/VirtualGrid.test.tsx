import { act, fireEvent, render, screen } from "@testing-library/react";
import { createRef } from "react";
import { describe, expect, it, vi } from "vitest";
import type { WorkSummary } from "@/api/schemas";
import { Artwork } from "@/components/Artwork";
import { type GridHandle, VirtualGrid } from "./VirtualGrid";

const work = (n: number): WorkSummary => ({
  id: `id-${n}`,
  kind: "movie",
  title: `Title ${n}`,
  matchState: "matched",
  available: true,
  images: {},
});

describe("VirtualGrid", () => {
  it("does not rebuild unchanged cards while arrow navigation settles", () => {
    vi.useFakeTimers();
    try {
      const renderCard = vi.fn((w: WorkSummary) => <a href={`/works/${w.id}`}>{w.title}</a>);
      const items = Array.from({ length: 30 }, (_, i) => work(i));
      const { container } = render(
        <VirtualGrid
          items={items}
          shape="poster"
          view="grid"
          onRangeChange={() => {}}
          renderCard={renderCard}
          renderListRow={() => null}
        />,
      );
      const link = container.querySelector("a");
      link?.focus();
      renderCard.mockClear();
      fireEvent.keyDown(link ?? document.body, { key: "ArrowRight" });
      act(() => vi.advanceTimersByTime(300));
      expect(document.activeElement).toHaveTextContent("Title 1");
      expect(renderCard).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  it("updates a changed item without rebuilding its unchanged neighbours", () => {
    const items = Array.from({ length: 20 }, (_, i) => work(i));
    const renderCard = vi.fn((w: WorkSummary) => <a href={`/works/${w.id}`}>{w.title}</a>);
    const props = {
      shape: "poster" as const,
      view: "grid" as const,
      renderCard,
      renderListRow: () => null,
      onRangeChange: () => {},
    };
    const view = render(<VirtualGrid {...props} items={items} />);
    renderCard.mockClear();
    view.rerender(<VirtualGrid {...props} items={items.map((w, i) => (i === 0 ? { ...w, title: "Updated" } : w))} />);
    expect(screen.getByText("Updated")).toBeInTheDocument();
    expect(renderCard).toHaveBeenCalledTimes(1);
  });

  it("renders only rows near the viewport", () => {
    const items = Array.from({ length: 5000 }, (_, i) => work(i));
    render(
      <VirtualGrid
        items={items}
        shape="poster"
        view="grid"
        onRangeChange={() => {}}
        renderCard={(w) => <span>{w.title}</span>}
        renderListRow={(w) => <span>{w.title}</span>}
      />,
    );
    expect(screen.getByText("Title 0")).toBeInTheDocument();
    expect(screen.queryByText("Title 4999")).not.toBeInTheDocument();
    expect(screen.getAllByText(/^Title /).length).toBeLessThan(200);
  });

  it("shows placeholders for unloaded items and reports the items it renders", () => {
    const onRangeChange = vi.fn();
    const items = [work(0), work(1), ...Array.from({ length: 998 }, () => undefined)];
    const { container } = render(
      <VirtualGrid
        items={items}
        shape="poster"
        view="grid"
        onRangeChange={onRangeChange}
        renderCard={(w) => <span>{w.title}</span>}
        renderListRow={(w) => <span>{w.title}</span>}
      />,
    );
    expect(container.querySelectorAll('[aria-hidden="true"]').length).toBeGreaterThan(0);
    const range = onRangeChange.mock.lastCall?.[0];
    expect(range.start).toBe(0);
    expect(range.end).toBeGreaterThan(2);
    expect(range.end).toBeLessThan(items.length);
    // The top row in view. The grid renders whole rows.
    expect(range.top).toBe(0);
    expect(range.end % range.topEnd).toBe(0);
  });

  it("asks for pages where it settles, not for the rows it scrolls past, and holds back their artwork", () => {
    vi.useFakeTimers();
    Object.defineProperty(document.documentElement, "scrollHeight", { value: 1_000_000, configurable: true });
    const onRangeChange = vi.fn();
    const items = Array.from({ length: 5000 }, (_, i) => ({
      ...work(i),
      images: { poster: { id: `image-${i}`, width: 780, height: 1170 } },
    }));
    const { container } = render(
      <VirtualGrid
        items={items}
        shape="poster"
        view="grid"
        onRangeChange={onRangeChange}
        renderCard={(w, width) => <Artwork image={w.images.poster} title={w.title} shape="poster" width={width} />}
        renderListRow={() => null}
      />,
    );
    const posters = () => container.querySelectorAll('img[src*="/images/"]').length;
    expect(posters()).toBeGreaterThan(0);
    act(() => {
      Object.defineProperty(window, "scrollY", { value: 50_000, configurable: true });
      window.dispatchEvent(new Event("scroll"));
    });
    // Mid-scroll, the pages asked for are still those at the top, and the
    // new rows show placeholders.
    expect(onRangeChange.mock.lastCall?.[0].start).toBe(0);
    expect(posters()).toBe(0);
    act(() => vi.advanceTimersByTime(200));
    expect(onRangeChange.mock.lastCall?.[0].start).toBeGreaterThan(0);
    expect(posters()).toBeGreaterThan(0);
    vi.useRealTimers();
    Reflect.deleteProperty(window, "scrollY");
    Reflect.deleteProperty(document.documentElement, "scrollHeight");
  });

  it("counts as moving while it scrolls, jumps, steps by arrow key or is held by the rail, holding back only the posters it brings in", () => {
    vi.useFakeTimers();
    Object.defineProperty(window, "scrollY", { value: 0, configurable: true });
    Object.defineProperty(document.documentElement, "scrollHeight", { value: 1_000_000, configurable: true });
    const onRangeChange = vi.fn();
    const items = Array.from({ length: 3000 }, (_, i) => ({
      ...work(i),
      images: { poster: { id: `moving-${i}`, width: 780, height: 1170 } },
    }));
    const grid = createRef<GridHandle>();
    const view = (scrubbing: boolean) => (
      <VirtualGrid
        ref={grid}
        items={items}
        shape="poster"
        view="grid"
        scrubbing={scrubbing}
        onRangeChange={onRangeChange}
        renderCard={(w, width) => (
          <a href={`/works/${w.id}`}>
            <Artwork image={w.images.poster} title={w.title} shape="poster" width={width} />
          </a>
        )}
        renderListRow={() => null}
      />
    );
    const { container, rerender } = render(view(false));
    const posters = () => container.querySelectorAll('img[src*="/images/"]').length;
    const settled = posters();
    // The top row's end is its width in cards.
    const columns = onRangeChange.mock.lastCall?.[0].topEnd;
    expect(settled).toBeGreaterThan(0);
    // The page already has these posters. Rows a scroll brings back in
    // still wait while the grid moves.
    for (const img of container.querySelectorAll('img[src*="/images/"]')) fireEvent.load(img);
    const pagesAsked = () => onRangeChange.mock.lastCall?.[0].end;
    const asked = pagesAsked();
    const scrollTo = (y: number) =>
      act(() => {
        Object.defineProperty(window, "scrollY", { value: y, configurable: true });
        window.dispatchEvent(new Event("scroll"));
      });

    // A scroll, far down and back.
    scrollTo(50_000);
    scrollTo(0);
    expect(posters()).toBe(0);
    expect(pagesAsked()).toBe(asked);
    act(() => vi.advanceTimersByTime(400));
    expect(posters()).toBe(settled);

    // A jump that doesn't scroll, as to a letter already in view. The
    // posters showing keep showing: hiding them would flash the grid.
    act(() => grid.current?.scrollToIndex(0));
    expect(posters()).toBe(settled);
    act(() => vi.advanceTimersByTime(400));
    expect(posters()).toBe(settled);

    // An arrow key held down. Each repeat moves focus a row down.
    const first = container.querySelector<HTMLElement>('[data-index="0"] a');
    first?.focus();
    for (let i = 0; i < 3; i++) {
      act(() => {
        fireEvent.keyDown(document.activeElement ?? document.body, { key: "ArrowDown", repeat: i > 0 });
        vi.advanceTimersByTime(50);
      });
      expect(posters()).toBe(settled);
      expect(pagesAsked()).toBe(asked);
    }
    expect(document.activeElement?.closest("[data-index]")?.getAttribute("data-index")).toBe(String(3 * columns));
    act(() => vi.advanceTimersByTime(400));
    expect(posters()).toBe(settled);

    // The letter rail, held still. The grid counts as moving until it's let
    // go.
    rerender(view(true));
    act(() => vi.advanceTimersByTime(1000));
    expect(posters()).toBe(settled);
    rerender(view(false));
    expect(posters()).toBe(settled);

    vi.useRealTimers();
    Reflect.deleteProperty(window, "scrollY");
    Reflect.deleteProperty(document.documentElement, "scrollHeight");
  });

  it("loads the rows a slow scroll brings into view as they come, and holds those a fast one passes", () => {
    vi.useFakeTimers();
    let clock = 0;
    vi.spyOn(performance, "now").mockImplementation(() => clock);
    Object.defineProperty(window, "scrollY", { value: 0, configurable: true });
    Object.defineProperty(document.documentElement, "scrollHeight", { value: 1_000_000, configurable: true });
    const items = Array.from({ length: 3000 }, (_, i) => ({
      ...work(i),
      images: { poster: { id: `speed-${i}`, width: 780, height: 1170 } },
    }));
    const { container } = render(
      <VirtualGrid
        items={items}
        shape="poster"
        view="grid"
        onRangeChange={() => {}}
        renderCard={(w, width) => <Artwork image={w.images.poster} title={w.title} shape="poster" width={width} />}
        renderListRow={() => null}
      />,
    );
    const posters = () => container.querySelectorAll('img[src*="/images/"]').length;
    /** Scrolls by `by` pixels a frame for `frames` frames of 16 ms. */
    const scroll = (by: number, frames: number) => {
      for (let i = 0; i < frames; i++) {
        act(() => {
          clock += 16;
          vi.advanceTimersByTime(16);
          Object.defineProperty(window, "scrollY", { value: window.scrollY + by, configurable: true });
          window.dispatchEvent(new Event("scroll"));
        });
      }
    };
    // About half a window a second. Each row's posters load as the row
    // comes into view.
    scroll(6, 200);
    expect(window.scrollY).toBe(1200);
    expect(posters()).toBeGreaterThan(0);
    // Ten windows a second. No posters load until the scroll stops.
    scroll(120, 30);
    expect(posters()).toBe(0);
    act(() => {
      clock += 400;
      vi.advanceTimersByTime(400);
    });
    expect(posters()).toBeGreaterThan(0);

    vi.useRealTimers();
    Reflect.deleteProperty(window, "scrollY");
    Reflect.deleteProperty(document.documentElement, "scrollHeight");
  });

  it("leaves the arrow keys past its edges to the page, and steps down into a shorter last row", () => {
    Object.defineProperty(window, "scrollY", { value: 0, configurable: true });
    const onRangeChange = vi.fn();
    const items = Array.from({ length: 10 }, (_, i) => work(i));
    const { container } = render(
      <VirtualGrid
        items={items}
        shape="poster"
        view="grid"
        onRangeChange={onRangeChange}
        renderCard={(w) => <a href={`/works/${w.id}`}>{w.title}</a>}
        renderListRow={() => null}
      />,
    );
    const columns = onRangeChange.mock.lastCall?.[0].topEnd;
    expect(columns).toBeGreaterThan(1);
    expect(columns).toBeLessThan(10);
    const press = (index: number, key: string) => {
      const link = container.querySelector<HTMLElement>(`[data-index="${index}"] a`);
      link?.focus();
      return fireEvent.keyDown(link ?? document.body, { key });
    };
    const focusedIndex = () => document.activeElement?.closest("[data-index]")?.getAttribute("data-index");
    // fireEvent.keyDown is false when the grid took the key.
    expect(press(1, "ArrowUp")).toBe(true);
    expect(press(0, "ArrowLeft")).toBe(true);
    expect(press(9, "ArrowRight")).toBe(true);
    expect(press(9, "ArrowDown")).toBe(true);
    expect(press(columns - 1, "ArrowDown")).toBe(false);
    expect(focusedIndex()).toBe("9");
    expect(press(1, "ArrowRight")).toBe(false);
    expect(focusedIndex()).toBe("2");
    Reflect.deleteProperty(window, "scrollY");
  });

  it("at the end, renders the last rows and reports no item past the last", () => {
    vi.useFakeTimers();
    Object.defineProperty(window, "scrollY", { value: 0, configurable: true });
    Object.defineProperty(document.documentElement, "scrollHeight", { value: 10_000_000, configurable: true });
    const onRangeChange = vi.fn();
    const items = Array.from({ length: 2000 }, (_, i) => work(i));
    const { container } = render(
      <VirtualGrid
        items={items}
        shape="poster"
        view="grid"
        onRangeChange={onRangeChange}
        renderCard={(w) => <span>{w.title}</span>}
        renderListRow={() => null}
      />,
    );
    const rows = () => container.querySelectorAll('[data-part="grid"] > div').length;
    const first = rows();
    act(() => {
      Object.defineProperty(window, "scrollY", { value: 5_000_000, configurable: true });
      window.dispatchEvent(new Event("scroll"));
    });
    act(() => vi.advanceTimersByTime(200));
    expect(screen.getByText("Title 1999")).toBeInTheDocument();
    expect(rows()).toBeLessThanOrEqual(first);
    const range = onRangeChange.mock.lastCall?.[0];
    expect(range.start).toBeGreaterThan(1900);
    expect(range.end).toBe(items.length);
    expect(range.topEnd).toBeLessThanOrEqual(items.length);
    vi.useRealTimers();
    Reflect.deleteProperty(window, "scrollY");
    Reflect.deleteProperty(document.documentElement, "scrollHeight");
  });
});
