#!/usr/bin/env python3
"""Writes pgs-sample.sup, the PGS subtitle file the Apple and Android tests read.

Six display sets on a 1920x1080 video, its pictures lines of block letters
drawn from the small font below:

  14.1 s  epoch start: a line
  15.7 s  the clear
  25.2 s  epoch start: another line
  27.0 s  a sign added above it, its object sent in two fragments
  28.6 s  the line alone, from the epoch's objects, nothing sent again
  30.0 s  the clear

Run it from anywhere; it writes the same bytes each time:

  python3 clients/scripts/gen-pgs-sample.py
"""

import pathlib
import struct

CLIENTS = pathlib.Path(__file__).resolve().parent.parent
OUTPUTS = [
    CLIENTS / "apple/KivoTests/Fixtures/pgs-sample.sup",
    CLIENTS / "android/src/androidTest/assets/pgs/pgs-sample.sup",
]

WIDTH, HEIGHT = 1920, 1080

# 5x7 block letters.
FONT = {
    " ": ["00000"] * 7,
    "A": ["01110", "10001", "10001", "11111", "10001", "10001", "10001"],
    "C": ["01110", "10001", "10000", "10000", "10000", "10001", "01110"],
    "D": ["11110", "10001", "10001", "10001", "10001", "10001", "11110"],
    "E": ["11111", "10000", "10000", "11110", "10000", "10000", "11111"],
    "F": ["11111", "10000", "10000", "11110", "10000", "10000", "10000"],
    "G": ["01110", "10001", "10000", "10111", "10001", "10001", "01111"],
    "I": ["11111", "00100", "00100", "00100", "00100", "00100", "11111"],
    "L": ["10000", "10000", "10000", "10000", "10000", "10000", "11111"],
    "N": ["10001", "11001", "10101", "10011", "10001", "10001", "10001"],
    "O": ["01110", "10001", "10001", "10001", "10001", "10001", "01110"],
    "R": ["11110", "10001", "10001", "11110", "10100", "10010", "10001"],
    "S": ["01111", "10000", "10000", "01110", "00001", "00001", "11110"],
    "T": ["11111", "00100", "00100", "00100", "00100", "00100", "00100"],
}

# Palette entries: index, Y, Cr, Cb, alpha (BT.709, limited range).
TRANSPARENT, FILL, OUTLINE, EDGE, SIGN = 0, 1, 2, 3, 4
PALETTE = [
    (TRANSPARENT, 16, 128, 128, 0),
    (FILL, 235, 128, 128, 255),     # white
    (OUTLINE, 16, 128, 128, 255),   # black
    (EDGE, 16, 128, 128, 128),      # half-transparent black
    (SIGN, 219, 138, 16, 255),      # yellow
]


def render(text, fill, scale, outline):
    """The text as rows of palette indexes: letters in [fill], outlined in black, with a half-transparent edge."""
    margin = outline + 1
    cols = len(text) * 6 - 1
    w, h = cols * scale + 2 * margin, 7 * scale + 2 * margin
    on = [[False] * w for _ in range(h)]
    for i, ch in enumerate(text):
        for gy, row in enumerate(FONT[ch]):
            for gx, bit in enumerate(row):
                if bit == "1":
                    for y in range(scale):
                        for x in range(scale):
                            on[margin + gy * scale + y][margin + (i * 6 + gx) * scale + x] = True

    def near(y, x, r):
        return any(on[yy][xx] for yy in range(max(0, y - r), min(h, y + r + 1)) for xx in range(max(0, x - r), min(w, x + r + 1)))

    rows = []
    for y in range(h):
        row = []
        for x in range(w):
            if on[y][x]:
                row.append(fill)
            elif near(y, x, outline):
                row.append(OUTLINE)
            elif near(y, x, outline + 1):
                row.append(EDGE)
            else:
                row.append(TRANSPARENT)
        rows.append(row)
    return rows


def rle(rows):
    """PGS's run-length coding, each line ended by 0 0."""
    out = bytearray()
    for row in rows:
        x = 0
        while x < len(row):
            color = row[x]
            n = 1
            while x + n < len(row) and row[x + n] == color and n < 16383:
                n += 1
            if color == 0:
                out += bytes([0, n]) if n < 64 else bytes([0, 0x40 | n >> 8, n & 0xFF])
            elif n <= 2:
                out += bytes([color]) * n
            else:
                out += bytes([0, 0x80 | n, color]) if n < 64 else bytes([0, 0xC0 | n >> 8, n & 0xFF, color])
            x += n
        out += b"\0\0"
    return bytes(out)


def segment(ms, kind, body):
    return b"PG" + struct.pack(">IIBH", ms * 90, 0, kind, len(body)) + body


def pcs(number, state, objects):
    """A composition: [objects] as (object id, window id, x, y)."""
    body = struct.pack(">HHBHBBBB", WIDTH, HEIGHT, 0x10, number, state, 0, 0, len(objects))
    for oid, wid, x, y in objects:
        body += struct.pack(">HBBHH", oid, wid, 0, x, y)
    return (0x16, body)


def wds(windows):
    body = bytes([len(windows)])
    for wid, x, y, w, h in windows:
        body += struct.pack(">BHHHH", wid, x, y, w, h)
    return (0x17, body)


def pds():
    return (0x14, bytes([0, 0]) + b"".join(bytes(entry) for entry in PALETTE))


def ods(oid, rows, fragments=1):
    data = rle(rows)
    w, h = len(rows[0]), len(rows)
    step = -(-len(data) // fragments)
    chunks = [data[i:i + step] for i in range(0, len(data), step)]
    out = []
    for i, chunk in enumerate(chunks):
        flags = (0x80 if i == 0 else 0) | (0x40 if i == len(chunks) - 1 else 0)
        head = struct.pack(">HBB", oid, 0, flags)
        if i == 0:
            head += struct.pack(">I", len(data) + 4)[1:] + struct.pack(">HH", w, h)
        out.append((0x15, head + chunk))
    return out


def display_set(ms, segments):
    return b"".join(segment(ms, kind, body) for kind, body in segments) + segment(ms, 0x80, b"")


def opaque(rows):
    alpha = {entry[0]: entry[4] for entry in PALETTE}
    return sum(1 for row in rows for c in row if alpha[c] != 0)


def main():
    first = render("A FIRST LINE", FILL, 6, 3)
    second = render("A SECOND LINE", FILL, 6, 3)
    sign = render("SIGN", SIGN, 12, 4)

    def bottom(rows):
        return (WIDTH - len(rows[0])) // 2, HEIGHT - 80 - len(rows)

    fx, fy = bottom(first)
    sx, sy = bottom(second)
    gx, gy = (WIDTH - len(sign[0])) // 2, 100
    line_window = (0, 0, sy - 20, WIDTH, HEIGHT - sy + 20)
    sign_window = (1, 0, gy, WIDTH, len(sign))

    sets = [
        display_set(14100, [pcs(0, 0x80, [(0, 0, fx, fy)]), wds([(0, 0, fy, WIDTH, HEIGHT - fy)]), pds(), *ods(0, first)]),
        display_set(15700, [pcs(1, 0x00, []), wds([(0, 0, fy, WIDTH, HEIGHT - fy)])]),
        display_set(25200, [pcs(2, 0x80, [(0, 0, sx, sy)]), wds([line_window, sign_window]), pds(), *ods(0, second)]),
        display_set(27000, [pcs(3, 0x00, [(0, 0, sx, sy), (1, 1, gx, gy)]), wds([line_window, sign_window]), pds(), *ods(1, sign, fragments=2)]),
        display_set(28600, [pcs(4, 0x00, [(0, 0, sx, sy)]), wds([line_window, sign_window])]),
        display_set(30000, [pcs(5, 0x00, []), wds([line_window, sign_window])]),
    ]
    data = b"".join(sets)
    for path in OUTPUTS:
        path.write_bytes(data)
    for name, rows, (x, y) in [("first", first, (fx, fy)), ("second", second, (sx, sy)), ("sign", sign, (gx, gy))]:
        print(f"{name}: at {x},{y}, {len(rows[0])}x{len(rows)}, {opaque(rows)} opaque pixels")
    print(f"{len(data)} bytes")


if __name__ == "__main__":
    main()
