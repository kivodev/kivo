import { encode } from "uqr";

/**
 * A QR code of value, drawn as SVG modules. It is black on white with a
 * quiet zone, so a camera reads it whatever the theme. label names it for
 * assistive technology.
 */
export function QrCode({ value, label, className }: { value: string; label: string; className?: string }) {
  const { data, size } = encode(value, { ecc: "M", border: 2 });
  let path = "";
  data.forEach((row, y) => {
    row.forEach((dark, x) => {
      if (dark) path += `M${x} ${y}h1v1h-1z`;
    });
  });
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox={`0 0 ${size} ${size}`}
      shapeRendering="crispEdges"
      className={`rounded-md bg-on-player fill-player ${className ?? ""}`}
    >
      <path d={path} />
    </svg>
  );
}
