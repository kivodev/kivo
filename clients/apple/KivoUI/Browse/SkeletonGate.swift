import SwiftUI

/// A screen's first load: nothing for its first 300 ms, so a
/// quick load goes straight to the content, then [skeleton], which once
/// shown stays at least 300 ms so it doesn't flicker. The content fades in.
/// Once loaded it stays: a refresh updates the content in place.
public struct SkeletonGate<Content: View, Skeleton: View>: View {
    private enum Stage {
        case waiting
        case skeleton
        case content
    }

    private static var delay: Duration { .milliseconds(300) }

    private let loaded: Bool
    private let content: () -> Content
    private let skeleton: () -> Skeleton
    /// When the skeleton showed, while it shows.
    @State private var skeletonSince: ContinuousClock.Instant?

    public init(loaded: Bool, @ViewBuilder content: @escaping () -> Content, @ViewBuilder skeleton: @escaping () -> Skeleton) {
        self.loaded = loaded
        self.content = content
        self.skeleton = skeleton
    }

    private var stage: Stage {
        if skeletonSince != nil { return .skeleton }
        return loaded ? .content : .waiting
    }

    public var body: some View {
        ZStack(alignment: .top) {
            switch stage {
            case .waiting: Color.clear.frame(height: 0)
            case .skeleton: skeleton()
            case .content: content()
            }
        }
        .fades(with: stage)
        .task(id: loaded) {
            let clock = ContinuousClock()
            if !loaded {
                guard skeletonSince == nil else { return }
                try? await clock.sleep(for: Self.delay)
                if !Task.isCancelled { skeletonSince = clock.now }
            } else if let since = skeletonSince {
                try? await clock.sleep(until: since.advanced(by: Self.delay))
                if !Task.isCancelled { skeletonSince = nil }
            }
        }
    }
}
