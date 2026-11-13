import CoreGraphics
import Foundation
import Libass

/// Draws a prepared ASS file styled, through libass with the file's fonts,
/// and the system's by CoreText for any glyph they lack. A frame renders
/// only when libass says it changed, or the canvas did; its images are
/// blended into one picture covering what they touch, in buffers reused
/// once Core Animation lets their last picture go.
final class ASSRenderer: SubtitleRenderer {
    private let library: OpaquePointer
    private let renderer: OpaquePointer
    private let track: UnsafeMutablePointer<ASS_Track>
    private var canvas: SubtitleCanvas?
    private let buffers = PixelBuffers()

    enum Failure: Error {
        case unreadable
    }

    /// [fonts] are each font's name, for libass's messages, and its file.
    init(script: Data, fonts: [(name: String, data: Data)]) throws {
        guard let library = ass_library_init() else { throw Failure.unreadable }
        ass_set_message_cb(library, { _, _, _, _ in }, nil)
        for font in fonts {
            font.data.withUnsafeBytes { bytes in
                ass_add_font(library, font.name, bytes.baseAddress?.assumingMemoryBound(to: CChar.self), Int32(bytes.count))
            }
        }
        guard let renderer = ass_renderer_init(library) else {
            ass_library_done(library)
            throw Failure.unreadable
        }
        ass_set_fonts(renderer, nil, "Arial", Int32(ASS_FONTPROVIDER_AUTODETECT.rawValue), nil, 1)
        var text = [CChar](script.map { CChar(bitPattern: $0) })
        guard let track = ass_read_memory(library, &text, text.count, nil) else {
            ass_renderer_done(renderer)
            ass_library_done(library)
            throw Failure.unreadable
        }
        self.library = library
        self.renderer = renderer
        self.track = track
    }

    deinit {
        ass_free_track(track)
        ass_renderer_done(renderer)
        ass_library_done(library)
    }

    func render(ms: Int64, canvas: SubtitleCanvas) -> [SubtitlePicture]? {
        let width = Int32((canvas.size.width * canvas.scale).rounded())
        let height = Int32((canvas.size.height * canvas.scale).rounded())
        guard width > 0, height > 0 else { return [] }
        let resized = canvas != self.canvas
        if resized {
            self.canvas = canvas
            ass_set_frame_size(renderer, width, height)
            ass_set_storage_size(renderer, Int32(canvas.video.width), Int32(canvas.video.height))
        }
        var change: Int32 = 0
        let first = ass_render_frame(renderer, track, ms, &change)
        guard change != 0 || resized else { return nil }

        var bounds: (minX: Int32, minY: Int32, maxX: Int32, maxY: Int32)?
        var image = first
        while let i = image {
            let s = i.pointee
            if s.w > 0, s.h > 0 {
                let b = bounds ?? (s.dst_x, s.dst_y, s.dst_x + s.w, s.dst_y + s.h)
                bounds = (min(b.minX, s.dst_x), min(b.minY, s.dst_y), max(b.maxX, s.dst_x + s.w), max(b.maxY, s.dst_y + s.h))
            }
            image = s.next
        }
        guard let bounds else { return [] }
        let x0 = Int(max(bounds.minX, 0))
        let y0 = Int(max(bounds.minY, 0))
        let w = Int(min(bounds.maxX, width)) - x0
        let h = Int(min(bounds.maxY, height)) - y0
        guard w > 0, h > 0 else { return [] }

        let stride = w * 4
        let buffer = buffers.take(count: stride * h)
        let pixels = buffer.bytes
        pixels.initialize(repeating: 0, count: stride * h)
        image = first
        while let i = image {
            Self.blend(i.pointee, into: pixels, stride: stride, x0: x0, y0: y0, w: w, h: h)
            image = i.pointee.next
        }
        guard let picture = buffers.image(buffer, width: w, height: h, stride: stride) else { return [] }
        let frame = CGRect(x: CGFloat(x0) / canvas.scale, y: CGFloat(y0) / canvas.scale, width: CGFloat(w) / canvas.scale, height: CGFloat(h) / canvas.scale)
        return [SubtitlePicture(image: picture, frame: frame)]
    }

    /// Blends one of libass's images, an alpha mask in one color, over the
    /// premultiplied pixels whose top left is [x0], [y0] on the frame.
    private static func blend(_ image: ASS_Image, into pixels: UnsafeMutablePointer<UInt8>, stride: Int, x0: Int, y0: Int, w: Int, h: Int) {
        let color = image.color
        let opacity = 255 - Int(color & 0xFF)
        guard opacity > 0, let bitmap = image.bitmap else { return }
        let r = Int(color >> 24 & 0xFF), g = Int(color >> 16 & 0xFF), b = Int(color >> 8 & 0xFF)
        let left = max(Int(image.dst_x), x0), right = min(Int(image.dst_x + image.w), x0 + w)
        let top = max(Int(image.dst_y), y0), bottom = min(Int(image.dst_y + image.h), y0 + h)
        guard left < right, top < bottom else { return }
        for y in top ..< bottom {
            let src = bitmap + (y - Int(image.dst_y)) * Int(image.stride) - Int(image.dst_x)
            var dst = pixels + (y - y0) * stride + (left - x0) * 4
            for x in left ..< right {
                let k = (Int(src[x]) * opacity + 127) / 255
                if k != 0 {
                    let rest = 255 - k
                    dst[0] = UInt8((r * k + Int(dst[0]) * rest + 127) / 255)
                    dst[1] = UInt8((g * k + Int(dst[1]) * rest + 127) / 255)
                    dst[2] = UInt8((b * k + Int(dst[2]) * rest + 127) / 255)
                    dst[3] = UInt8(k + (Int(dst[3]) * rest + 127) / 255)
                }
                dst += 4
            }
        }
    }
}

/// Pixel buffers for pictures, each taken again once the picture made of it
/// is let go: its data provider's release hands it back, or frees it once
/// the renderer is gone.
private final class PixelBuffers: @unchecked Sendable {
    final class Buffer {
        let bytes: UnsafeMutablePointer<UInt8>
        let capacity: Int
        weak var pool: PixelBuffers?

        init(capacity: Int, pool: PixelBuffers) {
            bytes = .allocate(capacity: capacity)
            self.capacity = capacity
            self.pool = pool
        }

        deinit {
            bytes.deallocate()
        }
    }

    private let lock = NSLock()
    private var free: [Buffer] = []

    /// A free buffer of at least [count] bytes, or a new one.
    func take(count: Int) -> Buffer {
        lock.lock()
        defer { lock.unlock() }
        if let i = free.firstIndex(where: { $0.capacity >= count }) {
            return free.remove(at: i)
        }
        // The smallest are let go for a larger one, keeping a few.
        if free.count > 2 { free.removeFirst() }
        return Buffer(capacity: count, pool: self)
    }

    /// An image of [buffer]'s first [height] rows, which holds it until let go.
    func image(_ buffer: Buffer, width: Int, height: Int, stride: Int) -> CGImage? {
        let held = Unmanaged.passRetained(buffer).toOpaque()
        guard let provider = CGDataProvider(dataInfo: held, data: buffer.bytes, size: stride * height, releaseData: { info, _, _ in
            guard let info else { return }
            let buffer = Unmanaged<Buffer>.fromOpaque(info).takeRetainedValue()
            buffer.pool?.give(buffer)
        }) else {
            Unmanaged<Buffer>.fromOpaque(held).release()
            return nil
        }
        return CGImage(
            width: width, height: height, bitsPerComponent: 8, bitsPerPixel: 32, bytesPerRow: stride,
            space: CGColorSpace(name: CGColorSpace.sRGB)!,
            bitmapInfo: CGBitmapInfo(rawValue: CGImageAlphaInfo.premultipliedLast.rawValue),
            provider: provider, decode: nil, shouldInterpolate: false, intent: .defaultIntent
        )
    }

    private func give(_ buffer: Buffer) {
        lock.lock()
        defer { lock.unlock() }
        free.append(buffer)
        free.sort { $0.capacity < $1.capacity }
    }
}
