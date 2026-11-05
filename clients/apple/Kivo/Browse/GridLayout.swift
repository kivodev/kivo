import KivoUI
import SwiftUI

/// The grid's columns for its width: three posters or squares across an
/// iPhone, two landscape cards, and as many as fit on wider screens, with
/// room on the trailing edge for the letter scrubber. A [wide] window's
/// cards are at least the shared tokens' widths, as the web's grids are.
struct GridLayout {
    let columns: [GridItem]
    let cardWidth: CGFloat
    let insets: EdgeInsets
    let rowSpacing: CGFloat
    /// Square cards say who wrote the book or how many books; others give the year when it fits.
    let caption: CardCaption

    /// [stacked] leaves room above each card for a series' stack of covers.
    init(width: CGFloat, style: CardStyle, scrubber: Bool, stacked: Bool = false, wide: Bool = false) {
        caption = style == .square ? .twoLines : .oneLine
        rowSpacing = KivoTokens.spacing * (stacked ? 6 : wide ? 5 : 4)
        let gap = wide ? KivoTokens.Layout.gridGap : KivoTokens.spacing * 2.5
        let leading = KivoTokens.spacing * 4
        let trailing = scrubber ? LetterScrubber.stripWidth + KivoTokens.spacing : leading
        let available = max(0, width - leading - trailing)
        let (fewest, narrowest): (Int, CGFloat) = switch (style, wide) {
        case (.landscape, false): (2, 170)
        case (_, false): (3, 104)
        case (.landscape, true): (2, KivoTokens.Layout.landscapeMinWidth)
        case (_, true): (3, KivoTokens.Layout.posterMinWidth)
        }
        let count = max(fewest, Int((available + gap) / (narrowest + gap)))
        cardWidth = floor((available - gap * CGFloat(count - 1)) / CGFloat(count))
        columns = Array(repeating: GridItem(.fixed(cardWidth), spacing: gap, alignment: .top), count: count)
        insets = EdgeInsets(top: stacked ? 10 : 0, leading: leading, bottom: 0, trailing: trailing)
    }
}
