"""Builds the player's fallback fonts for styled subtitles.

Usage: python3 fallback-fonts.py SRC OUT, with fontTools and brotli
installed, where SRC holds Google Fonts' variable fonts Arimo[wght].ttf and
NotoSansJP[wght].ttf (both OFL) as Arimo.ttf and NotoSansJP.ttf, and OUT is
src/features/player/fonts/.

kivo-fallback-latin-{400,700}.woff2: Arimo, metric-compatible with Arial,
at 400 and 700, subset to Latin, Greek and Cyrillic.
kivo-fallback-cjk.woff2: Noto Sans JP at 400, subset to kana, CJK
punctuation, full-width forms, the JIS X 0208 kanji and the GB 2312 level 1
hanzi it has, and no Latin.

Every file's family is renamed "Kivo Fallback": libass draws text in a font
a subtitle doesn't carry in one default family, choosing among its fonts by
the glyph, so Latin comes from Arimo and CJK from Noto Sans JP.
"""

import sys

from fontTools import subset
from fontTools.ttLib import TTFont
from fontTools.varLib import instancer

FAMILY = "Kivo Fallback"


def decoded(encoding, rows):
    chars = set()
    for hi in rows:
        for lo in range(0xA1, 0xFF):
            try:
                chars.add(ord(bytes([hi, lo]).decode(encoding)))
            except UnicodeDecodeError:
                pass
    return chars


def rename(font, style, weight):
    name = font["name"]
    full = FAMILY if style == "Regular" else f"{FAMILY} {style}"
    ps = (FAMILY + "-" + style).replace(" ", "")
    name.names = [n for n in name.names if n.nameID in (0, 5, 13, 14)]
    for pid, eid, lid in ((3, 1, 0x409), (1, 0, 0)):
        name.setName(FAMILY, 1, pid, eid, lid)
        name.setName(style, 2, pid, eid, lid)
        name.setName(f"{ps};kivo", 3, pid, eid, lid)
        name.setName(full, 4, pid, eid, lid)
        name.setName(ps, 6, pid, eid, lid)
    font["OS/2"].usWeightClass = weight
    if "STAT" in font:
        del font["STAT"]


def build(path, weight, unicodes, style, dest):
    font = instancer.instantiateVariableFont(TTFont(path), {"wght": weight})
    opts = subset.Options()
    opts.layout_features = ["*"]
    opts.name_IDs = ["*"]
    opts.name_languages = ["*"]
    opts.notdef_outline = True
    opts.hinting = False
    s = subset.Subsetter(opts)
    s.populate(unicodes=unicodes)
    s.subset(font)
    rename(font, style, weight)
    font.flavor = "woff2"
    font.save(dest)


def main(src, out):
    latin = (
        set(range(0x20, 0x7F)) | set(range(0xA0, 0x250)) | set(range(0x370, 0x530)) | set(range(0x1E00, 0x1F00))
        | set(range(0x2000, 0x2070)) | set(range(0x20A0, 0x20C0)) | {0x2122, 0x2190, 0x2192, 0x266A, 0x266B}
    )
    cjk = (
        set(range(0x3000, 0x3100)) | set(range(0x31F0, 0x3200)) | set(range(0xFF01, 0xFFF0))
        | decoded("euc_jp", range(0xB0, 0xF5)) | decoded("gb2312", range(0xB0, 0xD8))
    )
    build(f"{src}/Arimo.ttf", 400, latin, "Regular", f"{out}/kivo-fallback-latin-400.woff2")
    build(f"{src}/Arimo.ttf", 700, latin, "Bold", f"{out}/kivo-fallback-latin-700.woff2")
    build(f"{src}/NotoSansJP.ttf", 400, cjk, "Regular", f"{out}/kivo-fallback-cjk.woff2")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
