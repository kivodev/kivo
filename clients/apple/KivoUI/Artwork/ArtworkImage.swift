import KivoCore
import NukeUI
import SwiftUI

/// An image of a work, filling its frame: its average colour while it
/// loads, or the whole blurhash for a page's large artwork, the image fading
/// in over it unless it was already in memory or Reduce Motion is on, and a
/// titled tile in a neutral colour when there's no image.
public struct ArtworkImage: View {
    private let image: ImageRef?
    private let title: String
    private let pointWidth: CGFloat
    private let blur: Bool
    @Environment(\.artworkSource) private var source
    @Environment(\.displayScale) private var displayScale
    @Environment(\.kivoPalette) private var palette
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    /// [pointWidth] is about how wide it shows, for the size to fetch;
    /// [blur] shows the whole blurhash while it loads rather than its colour.
    public init(_ image: ImageRef?, title: String, pointWidth: CGFloat, blur: Bool = false) {
        self.image = image
        self.title = title
        self.pointWidth = pointWidth
        self.blur = blur
    }

    /// The images fill whatever frame the view is given: they're overlays on
    /// a clear view, which takes that frame, as a filled image would grow
    /// past it to keep its own aspect.
    public var body: some View {
        Color.clear
            .overlay { placeholder }
            .overlay {
                if let image, let request = source?.request(id: image.id, pixelWidth: pointWidth * displayScale) {
                    LazyImage(request: request, transaction: Transaction(animation: reduceMotion ? nil : .kivoFade)) { state in
                        if let loaded = state.image {
                            loaded.resizable().scaledToFill().transition(.opacity)
                        }
                    }
                }
            }
            .clipped()
        .accessibilityHidden(true)
    }

    @ViewBuilder private var placeholder: some View {
        if blur, let hash = image?.blurhash, let blurred = BlurHash.cached(hash) {
            Image(decorative: blurred, scale: 1).resizable().scaledToFill()
        } else if let hash = image?.blurhash, let color = BlurHash.averageColor(hash) {
            Color(cgColor: color)
        } else {
            ZStack {
                palette.placeholder
                if image == nil {
                    Text(title)
                        .kivoStyle(KivoTokens.TextSize.sm, weight: KivoTokens.Weight.medium)
                        .foregroundStyle(palette.onPlaceholder)
                        .multilineTextAlignment(.center)
                        .padding(KivoTokens.spacing * 2)
                }
            }
        }
    }
}
