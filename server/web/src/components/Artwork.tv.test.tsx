import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Artwork, ArtworkLoads, tintOf } from "./Artwork";

describe("Artwork on TV, with no artwork", () => {
  it.each(["poster", "square", "landscape"] as const)(
    "shows a tinted tile with its title and icon as a %s",
    (shape) => {
      const { container } = render(
        <Artwork image={undefined} title="Project Hail Mary" shape={shape} width={200} part="cover" />,
      );
      const tile = container.querySelector('[data-part="cover"]') as HTMLElement;
      expect(tile.className).toContain("bg-linear-to-");
      expect(tile.className).toContain("aspect-");
      expect(tile.querySelector("svg")).not.toBeNull();
      const title = screen.getByText("Project Hail Mary");
      expect(title.className).toContain("line-clamp-3");
      expect(title.className).toContain("text-balance");
    },
  );

  it("gives a title the same tint every time, and different titles a mix of them", () => {
    expect(tintOf("Dune")).toBe(tintOf("Dune"));
    const titles = ["Dune", "Arrival", "Alien", "Heat", "Ronin", "Up", "Her", "Tenet", "Sicario", "Coco"];
    expect(new Set(titles.map(tintOf)).size).toBeGreaterThan(1);
  });

  it("keeps a person's initials on a round tile", () => {
    render(<Artwork image={undefined} title="Andy Weir" shape="poster" width={120} corners="full" />);
    expect(screen.getByRole("img", { name: "Andy Weir" })).toHaveTextContent("AW");
  });
});

describe("TV artwork placeholders", () => {
  it("keeps started artwork through load gating and shows a cached image without another placeholder", () => {
    const image = { id: "kept-tv", blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj" };
    const art = <Artwork image={image} title="Kept" shape="poster" width={200} />;
    const view = render(<ArtworkLoads value>{art}</ArtworkLoads>);
    const loaded = screen.getByRole("img", { name: "Kept" });
    fireEvent.load(loaded);
    view.rerender(<ArtworkLoads value={false}>{art}</ArtworkLoads>);
    expect(screen.getByRole("img", { name: "Kept" })).toBe(loaded);
    view.unmount();
    const cached = render(art);
    expect(cached.container.querySelector('[data-part="artwork-placeholder"]')).toBeNull();
    expect(screen.getByRole("img", { name: "Kept" }).className).not.toContain("opacity-0");
  });

  it("removes the placeholder immediately for reduced motion", () => {
    vi.stubGlobal(
      "matchMedia",
      vi.fn(() => ({ matches: true })),
    );
    try {
      const view = render(
        <Artwork
          image={{ id: "reduced-tv", blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj" }}
          title="Reduced"
          shape="poster"
          width={200}
        />,
      );
      fireEvent.load(screen.getByRole("img", { name: "Reduced" }));
      expect(view.container.querySelector('[data-part="artwork-placeholder"]')).toBeNull();
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("uses the hash average colour without decoding or encoding a bitmap", () => {
    const canvas = vi.spyOn(HTMLCanvasElement.prototype, "getContext");
    const { container } = render(
      <Artwork
        image={{ id: "flat", blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj" }}
        title="Flat"
        shape="poster"
        width={200}
      />,
    );
    expect(canvas).not.toHaveBeenCalled();
    expect(container.querySelector('[data-part="artwork-placeholder"]')).toHaveStyle({
      backgroundColor: "rgb(151, 150, 149)",
    });
    expect(container.querySelector("img[aria-hidden]")).toBeNull();
  });

  it("keeps the placeholder through the fade and removes it only when opacity finishes", () => {
    const { container } = render(
      <Artwork
        image={{ id: "fade", blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj" }}
        title="Fade"
        shape="poster"
        width={200}
      />,
    );
    const image = screen.getByRole("img", { name: "Fade" });
    fireEvent.load(image);
    expect(container.querySelector('[data-part="artwork-placeholder"]')).not.toBeNull();
    fireEvent.transitionEnd(image, { propertyName: "transform" });
    expect(container.querySelector('[data-part="artwork-placeholder"]')).not.toBeNull();
    fireEvent.transitionEnd(image, { propertyName: "opacity" });
    expect(container.querySelector('[data-part="artwork-placeholder"]')).toBeNull();
  });

  it("shows the next image's colour while it loads, as a backdrop moves from work to work", () => {
    const hash = "LEHV6nWB2yk8pyo0adR*.7kCMdnj";
    const art = (id: string) => (
      <Artwork image={{ id, blurhash: hash }} title="Backdrop" shape="landscape" width={960} />
    );
    const { container, rerender } = render(art("backdrop-one"));
    const image = screen.getByRole("img", { name: "Backdrop" });
    fireEvent.load(image);
    fireEvent.transitionEnd(image, { propertyName: "opacity" });
    expect(container.querySelector('[data-part="artwork-placeholder"]')).toBeNull();
    rerender(art("backdrop-two"));
    expect(container.querySelector('[data-part="artwork-placeholder"]')).not.toBeNull();
    expect(screen.getByRole("img", { name: "Backdrop" }).className).toContain("opacity-0");
  });
});
