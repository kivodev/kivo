// The page's glide on TV: the window moved to a place over a few frames,
// which a press made during it re-aims rather than restarts, and which the
// TV's compositor runs, so nothing on the main thread can make it stutter.
//
// A TV's main thread is slow: each press of an arrow renders rows, moves
// focus and lays the page out again, for 50 to 100 ms on a 2025 Samsung.
// The browser's smooth scroll ran on the compositor but started each call
// over from rest, so a press during a glide slowed the page almost to a
// stop and sped it up again. A glide run from requestAnimationFrame kept
// its speed but stalled whenever the main thread was busy. So the page
// scrolls to where it's going at once, and the content is moved back by
// as much with a transform that then eases to nothing: a transition the
// compositor runs on its own. A press during a glide takes the content
// from where it shows at that moment, so the presses run on as one motion.
//
// The content is the elements in the page's main region that move with
// the page. One fixed to the screen, such as the letter rail, would move
// with a transformed ancestor, so the elements around it move instead.

/** How long a glide takes. */
const glideMs = 300;

/** A frame at 60 frames a second. */
const frameMs = 16;

/** An ease-out that starts fast, so a glide re-aimed during another keeps moving. */
const easing = "cubic-bezier(0.33, 1, 0.68, 1)";

/** Where the page is going: where it has scrolled to, as it scrolls at once. */
export function pageScrollTarget(): number {
  return window.scrollY;
}

/**
 * The elements a glide has moved. Each keeps its layer, and so its
 * will-change, until it's no longer one that moves: an element with
 * will-change: transform holds anything fixed inside it in place of the
 * window, so one fixed to the screen that shows up inside it later, as the
 * letter rail does in the content group Home glided, would stretch over the
 * whole page and scroll with it.
 */
const moved = new Set<HTMLElement>();

function release(el: HTMLElement) {
  el.style.willChange = "";
  el.style.transition = "";
  el.style.transform = "";
  moved.delete(el);
}

/**
 * Lets go of the elements a glide moved around `fixed`, an element fixed to
 * the screen, so it's fixed to the window again. It's called as one mounts,
 * before the next glide would find it.
 */
export function releaseAround(fixed: Element) {
  for (const el of moved) if (el.contains(fixed)) release(el);
}

/** The elements in root that move with the page, and none that's fixed to the screen. */
function movers(root: Element): HTMLElement[] {
  const out: HTMLElement[] = [];
  for (const child of Array.from(root.children)) {
    if (!(child instanceof HTMLElement) || child.classList.contains("fixed")) continue;
    if (child.querySelector(".fixed")) out.push(...movers(child));
    else out.push(child);
  }
  return out;
}

/** How far down a mover shows from where it is, as its transform has it now. */
function offsetOf(el: HTMLElement): number {
  // The computed transform is matrix(a, b, c, d, x, y), or matrix3d with y 14th.
  const transform = getComputedStyle(el).transform;
  const values = transform
    .slice(transform.indexOf("(") + 1, transform.lastIndexOf(")"))
    .split(",")
    .map(Number);
  const y = values.length === 16 ? values[13] : values.length === 6 ? values[5] : 0;
  return Number.isFinite(y) ? (y as number) : 0;
}

/**
 * Scrolls the page to `to`, gliding or at once. The page is at `to` at
 * once either way, so the rows there render; a glide only shows it moving
 * there. A glide under way is re-aimed from where the content shows.
 */
export function scrollPage(to: number, motion: "smooth" | "instant") {
  const max = Math.max(0, document.documentElement.scrollHeight - window.innerHeight);
  const target = Math.min(Math.max(to, 0), max);
  const root = document.getElementById("main") ?? document.querySelector("main");
  const moving = root ? movers(root) : [];
  for (const el of moved) if (!moving.includes(el)) release(el);
  for (const el of moving) moved.add(el);
  // Where the page shows now: where it's scrolled, less how far the content shows below that.
  const shown = window.scrollY - (moving[0] ? offsetOf(moving[0]) : 0);
  if (Math.abs(target - window.scrollY) >= 1) window.scrollTo({ top: target, behavior: "instant" });
  const from = motion === "smooth" ? target - shown : 0;
  for (const el of moving) {
    // Its own layer for as long as it moves with the page, so the compositor moves it without painting it again.
    el.style.willChange = "transform";
    el.style.transition = "none";
    el.style.transform = `translateY(${from}px)`;
  }
  if (from === 0 || moving.length === 0) return;
  // The start takes effect before the transition is set, so the content eases from there.
  void moving[0]?.offsetHeight;
  for (const el of moving) {
    // A new transition shows its start for a frame, where the page already
    // is, which would hold it still for that frame at each press; starting
    // it a frame in keeps it moving.
    el.style.transition = `transform ${glideMs}ms ${easing} -${frameMs}ms`;
    el.style.transform = "translateY(0px)";
  }
}
