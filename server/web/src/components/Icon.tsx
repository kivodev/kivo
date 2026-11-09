import { useId } from "react";

// Inline stroke icons on a 24-unit grid, drawn in currentColor so they take
// their colour from text tokens.
const paths = {
  home: "M3 10.5 12 3l9 7.5V21h-6v-6H9v6H3z",
  search: "M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14zm10 17-5-5",
  film: "M4 4h16v16H4zM8 4v16M16 4v16M4 8h4M4 12h4M4 16h4M16 8h4M16 12h4M16 16h4",
  tv: "M3 6h18v12H3zM8 21h8M12 18v3",
  video: "M3 6h13v12H3zM16 10l5-3v10l-5-3",
  folder: "M3 6h6l2 2h10v11H3z",
  libraries: "M4 4h4v16H4zM10 4h4v16h-4zM15.5 5.1l3.9-1 3.1 15-3.9 1z",
  book: "M5 4.5A1.5 1.5 0 0 1 6.5 3H20v15H6.5A1.5 1.5 0 0 0 5 19.5zM5 19.5A1.5 1.5 0 0 0 6.5 21H20v-3M9 7h7",
  play: "M7 4.5v15l12-7.5z",
  pause: "M8 5v14M16 5v14",
  replay: "M3 12a9 9 0 1 0 3-6.7M3 4v5h5",
  forward: "M21 12a9 9 0 1 1-3-6.7M21 4v5h-5",
  previous: "M6 5v14M18 5 9 12l9 7z",
  next: "M18 5v14M6 5l9 7-9 7z",
  fullscreen: "M4 9V4h5M15 4h5v5M20 15v5h-5M9 20H4v-5",
  exitFullscreen: "M9 4v5H4M20 9h-5V4M15 20v-5h5M4 15h5v5",
  pip: "M3 5h18v14H3zM12 12h7v5h-7z",
  volume: "M4 9h4l5-4v14l-5-4H4zM16.5 9a4 4 0 0 1 0 6M19 6.5a8 8 0 0 1 0 11",
  muted: "M4 9h4l5-4v14l-5-4H4zM16 9.5l5 5M21 9.5l-5 5",
  info: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 11v5M12 8h.01",
  captions: "M4 4h16v12H10l-5 4v-4H4zM8 9h8M8 12h5",
  arrowLeft: "M19 12H5M11 6l-6 6 6 6",
  spinner: "M12 3a9 9 0 1 0 9 9",
  check: "M5 12.5 10 17.5 19 7",
  checkCircle: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM8 12.5l3 3 5-6",
  more: "M5 12h.01M12 12h.01M19 12h.01",
  grip: "M9 6h.01M15 6h.01M9 12h.01M15 12h.01M9 18h.01M15 18h.01",
  chevronDown: "m6 9 6 6 6-6",
  chevronUp: "m6 15 6-6 6 6",
  chevronLeft: "m15 6-6 6 6 6",
  chevronRight: "m9 6 6 6-6 6",
  grid: "M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z",
  list: "M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01",
  close: "M6 6l12 12M18 6 6 18",
  user: "M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zm-8 9c0-4 3.6-6 8-6s8 2 8 6",
  alert: "M12 3 2 21h20zM12 10v5M12 18h.01",
  refresh: "M20 11a8 8 0 1 0-2.3 5.7M20 5v6h-6",
  signOut: "M15 4h4v16h-4M10 8l-4 4 4 4M6 12h11",
  network: "M10 3h4v5h-4zM3 16h4v5H3zM17 16h4v5h-4zM12 8v4M5 16v-4h14v4",
  calendar: "M4 5h16v16H4zM4 10h16M8 3v4M16 3v4",
  plus: "M12 5v14M5 12h14",
  pencil: "M4 20h4L19 9l-4-4L4 16zM13.5 6.5l4 4",
  lock: "M6 11h12v10H6zM8 11V7a4 4 0 0 1 8 0v4",
  key: "M8 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM12 12h9M17 12v3M21 12v3",
  shield: "M12 3 4 6v6c0 5 3.5 8 8 9 4.5-1 8-4 8-9V6z",
  gauge: "M4 18a8 8 0 1 1 16 0M12 18l4-6",
  activity: "M3 12h4l3-8 4 16 3-8h4",
  fileLines: "M6 3h8l4 4v14H6zM14 3v4h4M9 12h6M9 16h6",
  download: "M12 4v11M7 10l5 5 5-5M5 20h14",
  clock: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 7v5l3 3",
  chip: "M7 7h10v10H7zM9 3v4M15 3v4M9 17v4M15 17v4M3 9h4M3 15h4M17 9h4M17 15h4",
  screen: "M3 5h18v12H3zM10 8.5l5 2.5-5 2.5zM8 21h8",
  settings:
    "M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 0 0 2.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 0 0 1.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 0 0-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 0 0-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 0 0-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 0 0-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 0 0 1.066-2.573c-.94-1.543.826-3.31 2.37-2.37c1 .608 2.296.07 2.572-1.065zM9 12a3 3 0 1 0 6 0a3 3 0 0 0-6 0",
} as const;

// Filled icons, used for the on state of a stroke icon. The shape is
// filled in, with an optional stroke cut out of it.
const filled = {
  checkCircleFill: { fill: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z", cut: "M8 12.5l3 3 5-6" },
} as const satisfies Record<string, { fill: string; cut?: string }>;

export type IconName = keyof typeof paths | keyof typeof filled;

export function Icon({ name, className = "size-5" }: { name: IconName; className?: string }) {
  const mask = `icon-${useId().replace(/[^\w-]/g, "")}`;
  if (name in filled) {
    const shape: { fill: string; cut?: string } = filled[name as keyof typeof filled];
    return (
      <svg
        viewBox="0 0 24 24"
        fill="currentColor"
        stroke="currentColor"
        strokeWidth={2}
        strokeLinecap="round"
        strokeLinejoin="round"
        className={className}
        aria-hidden="true"
        focusable="false"
      >
        {shape.cut && (
          <mask id={mask}>
            <rect width="24" height="24" fill="white" stroke="none" />
            <path d={shape.cut} fill="none" stroke="black" />
          </mask>
        )}
        <path d={shape.fill} mask={shape.cut ? `url(#${mask})` : undefined} />
      </svg>
    );
  }
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={name === "more" || name === "grip" ? 3 : 2}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
      aria-hidden="true"
      focusable="false"
    >
      <path d={paths[name as keyof typeof paths]} />
    </svg>
  );
}
