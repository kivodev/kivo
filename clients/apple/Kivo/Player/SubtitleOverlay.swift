import KivoCore
import SwiftUI

/// The subtitle overlay over the player's surface, drawing [externalSubtitle]
/// in the rect the surface shows the video in, which follows a turn or a pinch.
struct SubtitleOverlay: UIViewRepresentable {
    let externalSubtitle: TrackShowExternal
    let model: PlayerModel

    func makeUIView(context: Context) -> SubtitleOverlayView {
        let surface = model.surface
        let view = SubtitleOverlayView(engine: model.engine) { [weak surface] view in
            guard let surface else { return .zero }
            return surface.convert(surface.playerLayer.videoRect, to: view)
        }
        context.coordinator.rect = surface.playerLayer.observe(\.videoRect) { [weak view] _, _ in
            DispatchQueue.main.async { view?.videoRectChanged() }
        }
        return view
    }

    func updateUIView(_ view: SubtitleOverlayView, context: Context) {
        let model = model
        view.show(externalSubtitle) { try await model.read($0) }
    }

    static func dismantleUIView(_ view: SubtitleOverlayView, coordinator: Coordinator) {
        view.show(nil) { _ in Data() }
        coordinator.rect = nil
    }

    func makeCoordinator() -> Coordinator { Coordinator() }

    /// The surface's video rect, followed while the overlay shows.
    final class Coordinator {
        var rect: NSKeyValueObservation?
    }
}
