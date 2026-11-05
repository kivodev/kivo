import AVFoundation
import KivoCore
import SwiftUI

/// Where the app draws subtitle files over the video: PGS pictures and
/// styled ASS, placed in the rect the video shows in. The stream's own
/// subtitle rendition AVPlayer draws, and a burned-in one is in the
/// picture, so neither comes here.
///
/// The app reports that it draws PGS and ASS files (AppModel.subtitles), so
/// the server sends those as files, ASS with its fonts, and streams plain
/// text. Where the overlay can't show, in Picture in Picture or on an
/// AirPlay device, the core's player asks for plays without subtitle files
/// (PlayerModel).
struct SubtitleOverlay: UIViewRepresentable {
    let externalSubtitle: TrackShowExternal
    let model: PlayerModel

    func makeUIView(context: Context) -> SubtitleOverlayView {
        SubtitleOverlayView(surface: model.surface, engine: model.engine)
    }

    func updateUIView(_ view: SubtitleOverlayView, context: Context) {
        let model = model
        let externalSubtitle = externalSubtitle
        switch externalSubtitle.codec {
        case "hdmv_pgs_subtitle":
            view.draw(externalSubtitle.url) {
                let data = try await model.read(externalSubtitle.url)
                return try PGSRenderer(PGSSubtitles(data: data))
            }
        case "ass":
            view.draw(([externalSubtitle.url] + externalSubtitle.fonts).joined(separator: "\n")) {
                async let script = model.read(externalSubtitle.url)
                // A font that can't be read gives way to the system's.
                let fonts = await withTaskGroup(of: (name: String, data: Data)?.self) { group in
                    for font in externalSubtitle.fonts {
                        group.addTask { try? await (font, model.read(font)) }
                    }
                    return await group.reduce(into: []) { fonts, font in font.map { fonts.append($0) } }
                }
                return try await ASSRenderer(script: script, fonts: fonts)
            }
        default:
            view.draw(nil) { nil }
        }
    }

    static func dismantleUIView(_ view: SubtitleOverlayView, coordinator: ()) {
        view.draw(nil) { nil }
    }
}

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

/// The overlay's view: its renderer draws the time the player shows, off
/// the main thread, on each step of a periodic time observer, which also
/// fires as the player seeks, pauses, starts and changes rate, and as the
/// video's rect changes with a turn or a pinch. A draw still running when
/// another is asked for runs once more after it, at the latest time.
final class SubtitleOverlayView: UIView {
    private weak var surface: PlayerSurfaceView?
    private let engine: AVPlayerEngine
    private let queue = DispatchQueue(label: "kivo.subtitles", qos: .userInteractive)
    private var key: String?
    private var loading: Task<Void, Never>?
    private var renderer: SubtitleRenderer?
    private var drawing = false
    private var again = false
    private var pictures: [CALayer] = []
    private var timeObserver: Any?
    private var rectObservation: NSKeyValueObservation?

    init(surface: PlayerSurfaceView, engine: AVPlayerEngine) {
        self.surface = surface
        self.engine = engine
        super.init(frame: .zero)
        isUserInteractionEnabled = false
        clipsToBounds = true
        timeObserver = engine.player.addPeriodicTimeObserver(forInterval: CMTime(value: 1, timescale: 60), queue: .main) { [weak self] _ in
            self?.tick()
        }
        rectObservation = surface.playerLayer.observe(\.videoRect) { [weak self] _, _ in
            DispatchQueue.main.async { self?.tick() }
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

    /// Draws the file [key] names with the renderer [make] loads off the main thread, or none for a nil key.
    func draw(_ key: String?, make: @escaping @Sendable () async throws -> SubtitleRenderer?) {
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

    /// The video's rect in this view, where the pictures go.
    private var videoRect: CGRect {
        guard let surface else { return .zero }
        return surface.convert(surface.playerLayer.videoRect, to: self)
    }

    private func tick() {
        guard let renderer, window != nil else { return }
        if drawing {
            again = true
            return
        }
        let rect = videoRect
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
