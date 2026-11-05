import AVFoundation
import KivoCore

/// The audio the core asks of the platform beyond the engine: the speed,
/// AVPlayer's default rate so each part keeps it, and the sleep timer's
/// fade, a ramp of the player's volume.
final class BookAudioOutput: NSObject, BookAudio {
    private let player: AVPlayer
    private var fade: Task<Void, Never>?

    init(player: AVPlayer) {
        self.player = player
    }

    func setSpeed(speed: Double) {
        player.defaultRate = Float(speed)
        if player.rate > 0 { player.rate = Float(speed) }
    }

    func fadeOut(durationMs: Int64) {
        fade?.cancel()
        let from = player.volume
        let steps = max(1, Int(durationMs / 100))
        fade = Task { @MainActor [player] in
            for step in 1 ... steps {
                try? await Task.sleep(for: .milliseconds(100))
                guard !Task.isCancelled else { return }
                player.volume = from * Float(steps - step) / Float(steps)
            }
        }
    }

    func restoreVolume() {
        fade?.cancel()
        fade = nil
        player.volume = 1
    }
}
