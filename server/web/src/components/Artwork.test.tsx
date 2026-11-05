import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Artwork, ArtworkLoads } from "./Artwork";

describe("Artwork", () => {
  const image = { id: "img1", blurhash: undefined } as never;

  it("keeps its placeholder when the image fails, and tries again when it's shown again", () => {
    const { unmount } = render(<Artwork image={image} title="Dune" shape="poster" width={120} />);
    const failed = screen.getByRole("img", { name: "Dune" });
    fireEvent.error(failed);
    // No broken image over the placeholder.
    expect(failed.className).toContain("opacity-0");
    expect(failed.parentElement?.className).toContain("bg-placeholder");
    unmount();

    render(<Artwork image={image} title="Dune" shape="poster" width={120} />);
    const again = screen.getByRole("img", { name: "Dune" });
    expect(again).not.toBe(failed);
    expect(again.getAttribute("src")).toBe(failed.getAttribute("src"));
    fireEvent.load(again);
    expect(again.className).not.toContain("opacity-0");
  });

  it("keeps its image while a grid moves on, and holds back one that hasn't loaded yet", () => {
    const grid = (loads: boolean) => (
      <ArtworkLoads value={loads}>
        <Artwork image={image} title="Dune" shape="poster" width={120} />
      </ArtworkLoads>
    );
    const shown = render(grid(true));
    const img = shown.container.querySelector("img[src]");
    shown.rerender(grid(false));
    expect(shown.container.querySelector("img[src]")).toBe(img);
    shown.unmount();

    const waiting = render(grid(false));
    expect(waiting.container.querySelector("img[src]")).toBeNull();
    waiting.rerender(grid(true));
    expect(waiting.container.querySelector("img[src]")).not.toBeNull();
  });

  it("holds what it's given, such as a progress bar, inside its rounded corners, with artwork or without", () => {
    for (const art of [image, undefined]) {
      const { container, unmount } = render(
        <Artwork image={art} title="Dune" shape="landscape" width={120} part="still">
          <span data-part="progress" />
        </Artwork>,
      );
      const frame = container.querySelector('[data-part="still"]');
      expect(frame?.className).toContain("rounded-md");
      expect(frame?.className).toContain("overflow-hidden");
      expect(container.querySelector('[data-part="progress"]')?.parentElement).toBe(frame);
      unmount();
    }
  });

  it("shows a plain titled tile with no artwork", () => {
    const { container } = render(
      <Artwork image={undefined} title="Project Hail Mary" shape="poster" width={200} part="cover" />,
    );
    const tile = container.querySelector('[data-part="cover"]') as HTMLElement;
    expect(tile.className).toContain("bg-placeholder");
    expect(tile.className).not.toContain("bg-linear-to-");
    expect(tile.querySelector("svg")).toBeNull();
    expect(screen.getByText("Project Hail Mary").className).toContain("text-sm");
  });
});
