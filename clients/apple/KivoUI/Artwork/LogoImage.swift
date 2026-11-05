import KivoCore
import NukeUI
import SwiftUI

/// A work's logo (its wordmark), fitted inside its frame, for a detail
/// page's hero. Shows nothing while it loads; [fallback] shows when there's
/// no logo or it can't be loaded.
public struct LogoImage<Fallback: View>: View {
    private let image: ImageRef?
    private let pointWidth: CGFloat
    private let fallback: Fallback
    @Environment(\.artworkSource) private var source
    @Environment(\.displayScale) private var displayScale

    public init(_ image: ImageRef?, pointWidth: CGFloat, @ViewBuilder fallback: () -> Fallback) {
        self.image = image
        self.pointWidth = pointWidth
        self.fallback = fallback()
    }

    public var body: some View {
        if let image, let request = source?.request(id: image.id, pixelWidth: pointWidth * displayScale) {
            LazyImage(request: request) { state in
                if let loaded = state.image {
                    loaded.resizable().scaledToFit()
                } else if state.error != nil {
                    fallback
                } else {
                    Color.clear
                }
            }
        } else {
            fallback
        }
    }
}
