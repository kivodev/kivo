import SwiftUI

/// What a card says under its artwork.
public enum CardCaption: Sendable {
    /// The title, then a second line: an episode's title and time left, a movie's year.
    case twoLines
    /// One line, for a grid: the title, with the year after it in secondary
    /// text when both fit.
    case oneLine
}

/// A caption's skeleton: a bar on each of its lines, in the lines' own
/// styles, so it's as tall as the caption of a card [width] wide.
struct CaptionSkeleton: View {
    let caption: CardCaption
    let width: CGFloat

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            switch caption {
            case .twoLines:
                Text(verbatim: " ").kivoStyle(KivoTokens.TextSize.sm, weight: KivoTokens.Weight.medium)
                    .skeletonLine(width: (width * 0.75).rounded())
                Text(verbatim: " ").kivoStyle(KivoTokens.TextSize.xs)
                    .skeletonLine(width: (width * 0.45).rounded())
            case .oneLine:
                Text(verbatim: " ").kivoStyle(KivoTokens.TextSize.xs, weight: KivoTokens.Weight.medium)
                    .skeletonLine(width: (width * 0.75).rounded())
            }
        }
        .frame(width: width, alignment: .leading)
    }
}
