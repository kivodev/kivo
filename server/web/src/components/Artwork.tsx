import { decode, isBlurhashValid } from "blurhash";
import { createContext, type ReactNode, use, useState } from "react";
import { imageUrl } from "@/api/images";
import type { ImageRef } from "@/api/schemas";
import { isTv } from "@/tv/flag";
import { LibraryKindIcon } from "./LibraryKindIcon";

export type ArtworkShape = "poster" | "landscape" | "square";

const shapes: Record<ArtworkShape, string> = {
  poster: "aspect-poster",
  landscape: "aspect-landscape",
  square: "aspect-square",
};

const corners = { md: "rounded-md", full: "rounded-full", none: "" } as const;

// The most blurhash placeholders and loaded image URLs remembered. The
// latest ones are kept, so a long grid scrolled end to end holds no more
// than this.
const remembered = 1000;

const blurCache = new Map<string, string | undefined>();

/** The image URLs this page has loaded lately, which the browser has at hand. */
const loadedImages = new Set<string>();

/** Adds key to cache as the latest entry, making room for it. */
function remember<K>(cache: Map<K, unknown> | Set<K>, key: K) {
  cache.delete(key);
  const oldest = cache.keys().next();
  if (cache.size >= remembered && !oldest.done) cache.delete(oldest.value);
}

/** The one canvas blurhashes are drawn on. */
let blurCanvas: HTMLCanvasElement | undefined;

/**
 * Whether the artwork inside may load yet. A grid holds artwork back on
 * the rows it scrolls past, and those rows keep their placeholder. This
 * applies even to artwork the page has loaded before. Artwork that may
 * load once keeps its image after.
 */
export const ArtworkLoads = createContext(true);

/**
 * The highest pixel density a card's artwork loads at, in pixels per CSS
 * pixel. Beyond 2x, as on a phone's 3x screen, a small card looks no
 * sharper, and its decoded image takes over twice the memory.
 */
export const cardDensity = 2;

const base83 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~";

/** The hash's average sRGB colour, without decoding its image or encoding a PNG. */
function averageColour(hash: string): string | undefined {
  if (!isBlurhashValid(hash).result) return undefined;
  let colour = 0;
  for (const c of hash.slice(2, 6)) {
    const digit = base83.indexOf(c);
    if (digit < 0) return undefined;
    colour = colour * 83 + digit;
  }
  return colour <= 0xffffff ? `#${colour.toString(16).padStart(6, "0")}` : undefined;
}

/** A TV tile's average colour, or a tiny blurhash image for the web. */
function blurhashUrl(hash: string): string | undefined {
  if (blurCache.has(hash)) return blurCache.get(hash);
  let url: string | undefined;
  try {
    if (isTv) {
      url = averageColour(hash);
    } else {
      const size = 32;
      blurCanvas ??= document.createElement("canvas");
      const canvas = blurCanvas;
      canvas.width = size;
      canvas.height = size;
      const ctx = canvas.getContext("2d");
      if (ctx) {
        const data = ctx.createImageData(size, size);
        data.data.set(decode(hash, size, size));
        ctx.putImageData(data, 0, 0);
        url = canvas.toDataURL();
      }
    }
  } catch {
    // A hash that doesn't decode gets no placeholder.
    url = undefined;
  }
  remember(blurCache, hash);
  blurCache.set(hash, url);
  return url;
}

// On TV, the tint of a tile with no artwork, fading to the placeholder colour. A
// title picks one of these by its hash, so a grid of covers with none shows
// a mix of tints, and each title keeps its own. The classes are whole
// strings so Tailwind finds them.
const tints = [
  "bg-linear-to-br from-accent/25 to-placeholder",
  "bg-linear-to-tr from-accent/25 to-placeholder",
  "bg-linear-to-br from-success/25 to-placeholder",
  "bg-linear-to-tr from-success/25 to-placeholder",
  "bg-linear-to-br from-warning/25 to-placeholder",
  "bg-linear-to-tr from-warning/25 to-placeholder",
] as const;

/** The tint a title always gets: its characters summed into the list. */
export function tintOf(title: string): string {
  let hash = 0;
  for (const c of title) hash = (hash * 31 + (c.codePointAt(0) ?? 0)) >>> 0;
  return tints[hash % tints.length] as string;
}

// The library kind a shape stands for when the caller names none.
const shapeKinds: Record<ArtworkShape, string> = { poster: "movies", square: "audiobooks", landscape: "videos" };

// The title's size grows with the tile, as a small tile can't fit three
// lines of a large one.
function titleSize(shape: ArtworkShape, width: number): string {
  if (shape === "landscape" || width < 120) return "text-sm";
  return width < 180 ? "text-base" : "text-lg";
}

/** The first letters of the first and last words: "Andy Weir" → "AW". */
function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  const first = words[0]?.charAt(0) ?? "";
  const last = words.length > 1 ? (words.at(-1)?.charAt(0) ?? "") : "";
  return (first + last).toUpperCase();
}

type Props = {
  image: ImageRef | undefined;
  /** Shown on the tile when there is no artwork, and as the image's alt text. */
  title: string;
  /** The library's kind, for the icon on a TV tile with no artwork. By default the shape's. */
  kind?: string;
  shape: ArtworkShape;
  /** The displayed width in CSS pixels, to pick the stored size. */
  width: number;
  /** The highest pixel density to load at, in pixels per CSS pixel. Defaults to the screen's. */
  density?: number;
  part?: string;
  /** Rounded corners by default. Fully round for people, none for a backdrop. */
  corners?: keyof typeof corners;
  /** Cropped to the shape by default. Contained for a logo. */
  fit?: "cover" | "contain";
  className?: string;
  /** Shown over the artwork and clipped to its corners, such as a progress bar along its bottom. */
  children?: ReactNode;
};

/**
 * Artwork that loads lazily over its blurhash. With no artwork it shows a
 * titled tile in a neutral colour, never a broken image. On TV that tile
 * has a soft tint picked by the title and the library kind's icon.
 */
export function Artwork({
  image,
  title,
  kind,
  shape,
  width,
  density,
  part,
  corners: round = "md",
  fit = "cover",
  className = "",
  children,
}: Props) {
  const [arrived, setArrived] = useState(false);
  const placeholderFor = () => !isTv || !image || !loadedImages.has(imageUrl(image, width, density));
  const [placeholder, setPlaceholder] = useState(placeholderFor);
  // Artwork given another image, as a hero's backdrop is as focus moves
  // from work to work, starts again: its colour shows until the new image
  // fades in over it. The same image at another size keeps showing.
  const [shown, setShown] = useState(image?.id);
  if (shown !== image?.id) {
    setShown(image?.id);
    setArrived(false);
    setPlaceholder(placeholderFor());
  }
  // Once the artwork may load, it keeps loading, or showing, while the grid
  // moves on: hiding it again would blank every card on each arrow press.
  const [started, setStarted] = useState(false);
  const loads = use(ArtworkLoads) || started;
  if (loads && !started) setStarted(true);
  const base = `relative overflow-hidden bg-placeholder ${corners[round]} ${shapes[shape]} ${className}`;
  if (!image && (!isTv || round === "full")) {
    // A person without a photo shows their initials.
    return (
      <div data-part={part} className={`${base} @container flex items-center justify-center p-3 text-center`}>
        {round === "full" ? (
          <span role="img" aria-label={title} className="text-initials font-semibold text-on-placeholder">
            {initials(title)}
          </span>
        ) : (
          <span className="line-clamp-3 text-sm font-medium text-on-placeholder">{title}</span>
        )}
        {children}
      </div>
    );
  }
  if (!image) {
    // The icon is a watermark behind the title, so it takes no room from it.
    return (
      <div
        data-part={part}
        data-placeholder=""
        className={`${base} ${tintOf(title)} flex items-center justify-center p-3 text-center`}
      >
        <LibraryKindIcon
          kind={kind ?? shapeKinds[shape]}
          className="absolute right-2 bottom-2 size-1/4 text-on-placeholder opacity-30"
        />
        <span
          className={`relative line-clamp-3 text-balance font-semibold text-on-placeholder ${titleSize(shape, width)}`}
        >
          {title}
        </span>
        {children}
      </div>
    );
  }
  const blur = placeholder && image.blurhash ? blurhashUrl(image.blurhash) : undefined;
  const src = imageUrl(image, width, density);
  // The image fades in over its blurhash as it arrives. An image this page
  // has loaded before, as on returning to a page, shows at once.
  const opacity = arrived ? "transition-opacity duration-fade ease-out" : loadedImages.has(src) ? "" : "opacity-0";
  return (
    <div data-part={part} className={base}>
      {blur &&
        (isTv ? (
          <div
            data-part="artwork-placeholder"
            aria-hidden="true"
            className="absolute inset-0"
            style={{ backgroundColor: blur }}
          />
        ) : (
          <img
            data-part="artwork-placeholder"
            src={blur}
            alt=""
            aria-hidden="true"
            className="absolute inset-0 size-full object-cover"
          />
        ))}
      {loads ? (
        <img
          src={src}
          alt={title}
          loading="lazy"
          decoding="async"
          onLoad={() => {
            remember(loadedImages, src);
            loadedImages.add(src);
            setArrived(true);
            if (isTv && window.matchMedia?.("(prefers-reduced-motion: reduce)").matches) setPlaceholder(false);
          }}
          onTransitionEnd={(event) => {
            if (isTv && event.propertyName === "opacity") setPlaceholder(false);
          }}
          className={`absolute inset-0 size-full ${fit === "contain" ? "object-contain" : "object-cover"} ${opacity}`}
        />
      ) : (
        <span role="img" aria-label={title} />
      )}
      {children}
    </div>
  );
}
