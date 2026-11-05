import AVFoundation
import AVKit
import SwiftUI

/// The video: a view whose layer is an AVPlayerLayer. The player model
/// owns it, so it outlives the player screen while Picture in Picture
/// plays from its layer.
final class PlayerSurfaceView: UIView {
    override static var layerClass: AnyClass { AVPlayerLayer.self }

    var playerLayer: AVPlayerLayer {
        // The layer class above makes it one.
        layer as! AVPlayerLayer
    }

    init(player: AVPlayer) {
        super.init(frame: .zero)
        backgroundColor = .black
        playerLayer.player = player
        playerLayer.videoGravity = .resizeAspect
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) {
        fatalError("PlayerSurfaceView is made in code")
    }
}

/// The model's surface in SwiftUI.
struct PlayerSurface: UIViewRepresentable {
    let view: PlayerSurfaceView

    func makeUIView(context: Context) -> PlayerSurfaceView { view }

    func updateUIView(_ view: PlayerSurfaceView, context: Context) {}
}
