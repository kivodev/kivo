import UIKit

/// The interface's orientation on a phone. The app delegate answers with
/// `allowed`, so turning the interface means narrowing it, asking the
/// scene, then widening it again once the phone itself turns: the
/// interface stays turned until then.
@MainActor
enum Orientation {
    static var allowed: UIInterfaceOrientationMask = .allButUpsideDown

    static func request(_ orientations: UIInterfaceOrientationMask) {
        guard UIDevice.current.userInterfaceIdiom == .phone, let scene else { return }
        allow(orientations, in: scene)
        scene.requestGeometryUpdate(.iOS(interfaceOrientations: orientations)) { _ in }
        widenOnTurning(from: UIDevice.current.orientation)
    }

    /// Follows the phone again, ending a request: the interface turns to
    /// how the phone is held at once.
    static func follow() {
        guard UIDevice.current.userInterfaceIdiom == .phone, let scene else { return }
        stopWatching()
        requests += 1
        allow(.allButUpsideDown, in: scene)
    }

    private static func allow(_ orientations: UIInterfaceOrientationMask, in scene: UIWindowScene) {
        allowed = orientations
        for window in scene.windows {
            window.rootViewController?.setNeedsUpdateOfSupportedInterfaceOrientations()
        }
    }

    /// Watching for the phone to turn from how it was held.
    private static var turning: (any NSObjectProtocol)?
    /// Counts the requests, so that a request's settling doesn't end a later one's.
    private static var requests = 0

    /// UIKit keeps the narrowed orientations until it's told they changed,
    /// and told at once it would turn the interface straight back to how
    /// the phone is held. So it's told once the interface has turned, a
    /// second on, and the phone has turned from how it was [held], even
    /// if it did so in that second: the interface follows it from then on.
    /// Held flat or not yet known, the first way it's held counts as [held].
    private static func widenOnTurning(from held: UIDeviceOrientation) {
        var held = held
        stopWatching()
        requests += 1
        let request = requests
        var turned = false
        var settled = false
        let widen = {
            stopWatching()
            if let scene { allow(.allButUpsideDown, in: scene) }
        }
        UIDevice.current.beginGeneratingDeviceOrientationNotifications()
        turning = NotificationCenter.default.addObserver(forName: UIDevice.orientationDidChangeNotification, object: nil, queue: .main) { _ in
            MainActor.assumeIsolated {
                let now = UIDevice.current.orientation
                guard now.isValidInterfaceOrientation else { return }
                guard held.isValidInterfaceOrientation else { held = now; return }
                guard now != held else { return }
                turned = true
                if settled { widen() }
            }
        }
        Task {
            try? await Task.sleep(for: .seconds(1))
            guard request == requests else { return }
            settled = true
            allowed = .allButUpsideDown
            if turned { widen() }
        }
    }

    private static func stopWatching() {
        guard let turning else { return }
        NotificationCenter.default.removeObserver(turning)
        UIDevice.current.endGeneratingDeviceOrientationNotifications()
        self.turning = nil
    }

    /// Whether a phone's screen of [size] is an unfolded phone's inner
    /// one, which folds across its middle: no phone's screen is this
    /// big otherwise.
    static func unfolded(_ size: CGSize) -> Bool {
        UIDevice.current.userInterfaceIdiom == .phone && min(size.width, size.height) >= 600
    }

    private static var scene: UIWindowScene? {
        UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first
    }
}

