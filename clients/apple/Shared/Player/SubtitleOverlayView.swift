import AVFoundation
import KivoCore
import UIKit

/// What a renderer draws into: the video's rect, [size] points at [scale]
/// pixels a point, of a video [video] pixels in size.
struct SubtitleCanvas: Equatable {
    var size: CGSize
    var scale: CGFloat
    var video: CGSize
}

/// A picture to show, placed in the canvas's points.
struct SubtitlePicture {
    let image: CGImage
    let frame: CGRect
}

/// Draws a subtitle file at a time. A renderer is called on one
/// background queue at a time, never the main thread's.
protocol SubtitleRenderer: AnyObject {
    /// The pictures showing at [ms], or nil when they're those of the last call.
    func render(ms: Int64, canvas: SubtitleCanvas) -> [SubtitlePicture]?
}

/// Where the app draws subtitle files over the video: PGS pictures and
/// styled ASS, placed in the rect the video shows in. The stream's own
/// subtitle rendition AVPlayer draws, and a burned-in one is in the
/// picture, so neither comes here.
///
/// The app reports that it draws PGS and ASS files ([shown]), so
/// the server sends those as files, ASS with its fonts, and streams plain
/// text. Where the overlay can't show, as in Picture in Picture or on an
/// AirPlay device, the core's player asks for plays without subtitle files
/// (VideoPlayback.overlayVisible).
///
/// Its renderer draws the time the player shows, off the main thread, on
/// each step of a periodic time observer, which also fires as the player
/// seeks, pauses, starts and changes rate, and as the video's rect changes
/// ([videoRectChanged]). A draw still running when another is asked for
/// runs once more after it, at the latest time.
final class SubtitleOverlayView: UIView {
    /// The subtitles an app with the overlay shows, for the capability
    /// report: plain text as the stream's renditions, which AVPlayer draws,
    /// and PGS and styled ASS as files the overlay draws.
    static let shown = [
        SubtitleEntry(codec: "webvtt", delivery: ["stream"], containers: nil),
        SubtitleEntry(codec: "ass", delivery: ["external"], containers: nil),
        SubtitleEntry(codec: "hdmv_pgs_subtitle", delivery: ["external"], containers: nil),
    ]

    private let engine: AVPlayerEngine
    /// The video's rect in this view, where the pictures go.
    private let videoRect: (SubtitleOverlayView) -> CGRect
    private let queue = DispatchQueue(label: "kivo.subtitles", qos: .userInteractive)
    private var key: String?
    private var loading: Task<Void, Never>?
    private var renderer: SubtitleRenderer?
    private var drawing = false
    private var again = false
    private var pictures: [CALayer] = []
    private var timeObserver: Any?

    init(engine: AVPlayerEngine, videoRect: @escaping (SubtitleOverlayView) -> CGRect) {
        self.engine = engine
        self.videoRect = videoRect
        super.init(frame: .zero)
        isUserInteractionEnabled = false
        clipsToBounds = true
        timeObserver = engine.player.addPeriodicTimeObserver(forInterval: CMTime(value: 1, timescale: 60), queue: .main) { [weak self] _ in
            self?.tick()
        }
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) {
        fatalError("SubtitleOverlayView is made in code")
    }

    deinit {
        if let timeObserver { engine.player.removeTimeObserver(timeObserver) }
        loading?.cancel()
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        tick()
    }

    /// The video's rect changed, as with a turn or a pinch.
    func videoRectChanged() {
        tick()
    }

    /// Draws [subtitle], a PGS or styled ASS file with its fonts, each read
    /// by [read]; nil, or a file of another codec, draws nothing.
    func show(_ subtitle: TrackShowExternal?, read: @escaping @Sendable (String) async throws -> Data) {
        guard let subtitle else { return draw(nil) { nil } }
        switch subtitle.codec {
        case "hdmv_pgs_subtitle":
            draw(subtitle.url) {
                try PGSRenderer(PGSSubtitles(data: try await read(subtitle.url)))
            }
        case "ass":
            draw(([subtitle.url] + subtitle.fonts).joined(separator: "\n")) {
                async let script = read(subtitle.url)
                // A font that can't be read gives way to the system's.
                let fonts = await withTaskGroup(of: (name: String, data: Data)?.self) { group in
                    for font in subtitle.fonts {
                        group.addTask { try? await (font, read(font)) }
                    }
                    return await group.reduce(into: []) { fonts, font in font.map { fonts.append($0) } }
                }
                return try await ASSRenderer(script: script, fonts: fonts)
            }
        default:
            draw(nil) { nil }
        }
    }

    /// Draws the file [key] names with the renderer [make] loads off the main thread, or none for a nil key.
    private func draw(_ key: String?, make: @escaping @Sendable () async throws -> SubtitleRenderer?) {
        guard key != self.key else { return }
        self.key = key
        loading?.cancel()
        renderer = nil
        apply([])
        guard key != nil else { return }
        loading = Task { [weak self] in
            // A file that can't be read draws nothing.
            guard let made = try? await make(), !Task.isCancelled, let self, self.key == key else { return }
            self.renderer = made
            self.tick()
        }
    }

    private func tick() {
        guard let renderer, window != nil else { return }
        if drawing {
            again = true
            return
        }
        let rect = videoRect(self)
        let video = engine.player.currentItem?.presentationSize ?? .zero
        guard rect.width > 0, rect.height > 0, video.width > 0 else { return }
        let canvas = SubtitleCanvas(size: rect.size, scale: window?.screen.scale ?? 2, video: video)
        let ms = engine.positionMs
        drawing = true
        queue.async { [weak self] in
            let drawn = renderer.render(ms: ms, canvas: canvas)
            DispatchQueue.main.async {
                guard let self else { return }
                self.drawing = false
                if let drawn, renderer === self.renderer {
                    self.apply(drawn.map { SubtitlePicture(image: $0.image, frame: $0.frame.offsetBy(dx: rect.minX, dy: rect.minY)) })
                }
                if self.again {
                    self.again = false
                    self.tick()
                }
            }
        }
    }

    /// Shows [drawn], one layer each, reusing the layers there are.
    private func apply(_ drawn: [SubtitlePicture]) {
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        while pictures.count < drawn.count {
            let picture = CALayer()
            picture.contentsGravity = .resize
            layer.addSublayer(picture)
            pictures.append(picture)
        }
        for (i, picture) in pictures.enumerated() {
            if i < drawn.count {
                picture.contents = drawn[i].image
                picture.frame = drawn[i].frame
                picture.isHidden = false
            } else {
                picture.contents = nil
                picture.isHidden = true
            }
        }
        CATransaction.commit()
    }
}

/// Draws a PGS file's compositions, scaled from the PGS video's size to the canvas.
final class PGSRenderer: SubtitleRenderer {
    private let subtitles: PGSSubtitles
    private var shown: (index: Int?, canvas: SubtitleCanvas)?
    /// The shown composition's images, made once.
    private var images: [CGImage?] = []

    init(_ subtitles: PGSSubtitles) {
        self.subtitles = subtitles
    }

    func render(ms: Int64, canvas: SubtitleCanvas) -> [SubtitlePicture]? {
        let index = subtitles.composition(atMs: ms)
        if let shown, shown.index == index, shown.canvas == canvas { return nil }
        guard let index, subtitles.width > 0, subtitles.height > 0 else {
            shown = (index, canvas)
            images = []
            return []
        }
        let objects = subtitles.compositions[index].objects
        if shown?.index != index {
            images = objects.map { $0.picture.image(crop: $0.crop) }
        }
        shown = (index, canvas)
        let sx = canvas.size.width / CGFloat(subtitles.width)
        let sy = canvas.size.height / CGFloat(subtitles.height)
        return zip(objects, images).compactMap { object, image in
            guard let image else { return nil }
            let frame = CGRect(x: CGFloat(object.x) * sx, y: CGFloat(object.y) * sy, width: CGFloat(image.width) * sx, height: CGFloat(image.height) * sy)
            return SubtitlePicture(image: image, frame: frame)
        }
    }
}
