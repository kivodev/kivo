import CoreGraphics
import Foundation

/// A PGS file (`.sup`, Blu-ray's picture subtitles) as the compositions it
/// shows: each from its start until the next one's, its pictures placed on
/// the [width] × [height] video. A display set's segments are its
/// composition (PCS), windows (WDS), palettes (PDS) and objects (ODS),
/// ended by END. An epoch starts with empty palettes and objects; within it
/// a display set may update only some of them and use the rest as they
/// were.
struct PGSSubtitles {
    var width = 0
    var height = 0
    /// In time order; one with no objects clears the screen.
    private(set) var compositions: [Composition] = []

    struct Composition {
        let startMs: Int64
        let objects: [PlacedObject]
    }

    /// An object where its composition shows it: at [x], [y] on the video,
    /// cut to [crop] in the object's own pixels when cropped. [forced]
    /// marks one a forced-only track would show.
    struct PlacedObject {
        let x: Int
        let y: Int
        let crop: CGRect?
        let forced: Bool
        let picture: Picture
    }

    /// An object's run-length coded pixels and the palette it's shown in.
    struct Picture {
        let width: Int
        let height: Int
        let rle: Data
        /// 256 premultiplied RGBA colors, R in the low byte.
        let palette: [UInt32]
    }

    enum Failure: Error {
        case malformed(offset: Int)
    }

    init(data: Data) throws {
        let bytes = [UInt8](data)
        var palettes: [UInt8: [UInt32]] = [:]
        var objects: [UInt16: (width: Int, height: Int, rle: Data)] = [:]
        var pending: [UInt16: (width: Int, height: Int, rle: Data)] = [:]
        var display: (ms: Int64, paletteId: UInt8, refs: [(id: UInt16, x: Int, y: Int, crop: CGRect?, forced: Bool)])?
        var at = 0
        while at + 13 <= bytes.count {
            guard bytes[at] == 0x50, bytes[at + 1] == 0x47 else { throw Failure.malformed(offset: at) }
            let pts = Int64(bytes.u32(at + 2))
            let type = bytes[at + 10]
            let size = Int(bytes.u16(at + 11))
            let start = at + 13
            guard start + size <= bytes.count else { throw Failure.malformed(offset: at) }
            let body = bytes[start ..< start + size]
            at = start + size
            switch type {
            case 0x16 where size >= 11:
                let b = body.startIndex
                width = Int(bytes.u16(b))
                height = Int(bytes.u16(b + 2))
                if bytes[b + 7] & 0x80 != 0 {
                    // An epoch starts: nothing earlier is used again.
                    palettes = [:]
                    objects = [:]
                    pending = [:]
                }
                var refs: [(id: UInt16, x: Int, y: Int, crop: CGRect?, forced: Bool)] = []
                var p = b + 11
                for _ in 0 ..< Int(bytes[b + 10]) {
                    guard p + 8 <= body.endIndex else { break }
                    let flags = bytes[p + 3]
                    var crop: CGRect?
                    if flags & 0x80 != 0, p + 16 <= body.endIndex {
                        crop = CGRect(x: Int(bytes.u16(p + 8)), y: Int(bytes.u16(p + 10)), width: Int(bytes.u16(p + 12)), height: Int(bytes.u16(p + 14)))
                    }
                    refs.append((bytes.u16(p), Int(bytes.u16(p + 4)), Int(bytes.u16(p + 6)), crop, flags & 0x40 != 0))
                    p += flags & 0x80 != 0 ? 16 : 8
                }
                display = (pts / 90, bytes[b + 9], refs)
            case 0x14 where size >= 2:
                let b = body.startIndex
                var palette = palettes[bytes[b]] ?? Array(repeating: 0, count: 256)
                var p = b + 2
                while p + 5 <= body.endIndex {
                    palette[Int(bytes[p])] = Self.rgba(y: bytes[p + 1], cr: bytes[p + 2], cb: bytes[p + 3], alpha: bytes[p + 4])
                    p += 5
                }
                palettes[bytes[b]] = palette
            case 0x15 where size >= 4:
                let b = body.startIndex
                let id = bytes.u16(b)
                let sequence = bytes[b + 3]
                if sequence & 0x80 != 0 {
                    // The first fragment: the data's length, three bytes, then the size.
                    guard size >= 11 else { break }
                    pending[id] = (Int(bytes.u16(b + 7)), Int(bytes.u16(b + 9)), Data(bytes[(b + 11)...].prefix(size - 11)))
                } else {
                    pending[id]?.rle.append(contentsOf: body.dropFirst(4))
                }
                if sequence & 0x40 != 0, let object = pending.removeValue(forKey: id) {
                    objects[id] = object
                }
            case 0x80:
                guard let shown = display else { break }
                display = nil
                let palette = palettes[shown.paletteId] ?? Array(repeating: 0, count: 256)
                let placed = shown.refs.compactMap { ref -> PlacedObject? in
                    guard let object = objects[ref.id] else { return nil }
                    let picture = Picture(width: object.width, height: object.height, rle: object.rle, palette: palette)
                    return PlacedObject(x: ref.x, y: ref.y, crop: ref.crop, forced: ref.forced, picture: picture)
                }
                // A display set at the time of the last replaces it.
                if compositions.last?.startMs == shown.ms { compositions.removeLast() }
                compositions.append(Composition(startMs: shown.ms, objects: placed))
            default:
                break
            }
        }
        compositions.sort { $0.startMs < $1.startMs }
    }

    /// The composition showing at [ms]: the last to start by then.
    func composition(atMs ms: Int64) -> Int? {
        var low = 0
        var high = compositions.count
        while low < high {
            let mid = (low + high) / 2
            if compositions[mid].startMs <= ms { low = mid + 1 } else { high = mid }
        }
        return low > 0 ? low - 1 : nil
    }

    /// A palette entry in premultiplied RGBA, by BT.709's limited range.
    static func rgba(y: UInt8, cr: UInt8, cb: UInt8, alpha: UInt8) -> UInt32 {
        let l = 1.164 * (Double(y) - 16)
        let v = Double(cr) - 128
        let u = Double(cb) - 128
        let a = Double(alpha) / 255
        func channel(_ value: Double) -> UInt32 { UInt32(max(0, min(255, value)) * a + 0.5) }
        let r = channel(l + 1.793 * v)
        let g = channel(l - 0.213 * u - 0.533 * v)
        let b = channel(l + 2.112 * u)
        return r | g << 8 | b << 16 | UInt32(alpha) << 24
    }
}

extension PGSSubtitles.Picture {
    /// The picture's pixels, decoding its runs: a byte that isn't 0 is one
    /// pixel of that color; 0 then a flag byte is the line's end (0), or
    /// a run of color 0 or of the color after, 6 or 14 bits long.
    func pixels() -> [UInt32] {
        var out = [UInt32](repeating: 0, count: width * height)
        guard width > 0, height > 0 else { return out }
        rle.withUnsafeBytes { (raw: UnsafeRawBufferPointer) in
            let src = raw.bindMemory(to: UInt8.self)
            out.withUnsafeMutableBufferPointer { dst in
                var i = 0
                var line = 0
                var x = 0
                func put(_ count: Int, _ color: UInt8) {
                    guard line < height else { return }
                    let n = min(count, width - x)
                    guard n > 0 else { return }
                    let value = palette[Int(color)]
                    let base = line * width + x
                    for k in 0 ..< n { dst[base + k] = value }
                    x += n
                }
                while i < src.count, line < height {
                    let byte = src[i]
                    i += 1
                    if byte != 0 {
                        put(1, byte)
                        continue
                    }
                    guard i < src.count else { break }
                    let flag = src[i]
                    i += 1
                    if flag == 0 {
                        line += 1
                        x = 0
                        continue
                    }
                    var count = Int(flag & 0x3F)
                    if flag & 0x40 != 0, i < src.count {
                        count = count << 8 | Int(src[i])
                        i += 1
                    }
                    var color: UInt8 = 0
                    if flag & 0x80 != 0, i < src.count {
                        color = src[i]
                        i += 1
                    }
                    put(count, color)
                }
            }
        }
        return out
    }

    /// The picture as an image, cut to [crop] when given.
    func image(crop: CGRect? = nil) -> CGImage? {
        guard width > 0, height > 0 else { return nil }
        let data = pixels().withUnsafeBufferPointer { Data(buffer: $0) }
        guard let provider = CGDataProvider(data: data as CFData),
              let image = CGImage(
                  width: width, height: height, bitsPerComponent: 8, bitsPerPixel: 32, bytesPerRow: width * 4,
                  space: CGColorSpace(name: CGColorSpace.sRGB)!,
                  bitmapInfo: CGBitmapInfo(rawValue: CGImageAlphaInfo.premultipliedLast.rawValue),
                  provider: provider, decode: nil, shouldInterpolate: true, intent: .defaultIntent
              )
        else { return nil }
        guard let crop else { return image }
        return image.cropping(to: crop)
    }
}

private extension [UInt8] {
    func u16(_ at: Int) -> UInt16 { UInt16(self[at]) << 8 | UInt16(self[at + 1]) }
    func u32(_ at: Int) -> UInt32 { UInt32(u16(at)) << 16 | UInt32(u16(at + 2)) }
}
