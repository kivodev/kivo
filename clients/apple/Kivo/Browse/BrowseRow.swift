import KivoCore
import KivoUI
import SwiftUI

/// A heading over a horizontal row of cards, as Home's rows are. With [more], the heading is a button with a chevron that opens
/// everything the row starts.
struct BrowseRow: View {
    let title: String
    let items: [WorkSummary]
    /// The shape every card takes, nil where each work's own decides.
    var style: CardStyle?
    var more: (() -> Void)?
    @Environment(\.kivoPalette) private var palette
    @Environment(\.wide) private var wide

    static let headingSpacing = KivoTokens.spacing * 2.5
    static let inset = KivoTokens.spacing * 4

    /// A card's width in a row: a wide window's are the shared tokens' grid widths.
    static func cardWidth(_ style: CardStyle, wide: Bool = false) -> CGFloat {
        switch (style, wide) {
        case (.landscape, false): 260
        case (_, false): 116
        case (.landscape, true): KivoTokens.Layout.landscapeMinWidth * 1.25
        case (_, true): KivoTokens.Layout.posterMinWidth
        }
    }

    static func heading(_ title: String) -> some View {
        Text(title).kivoStyle(KivoTokens.TextSize.lg, weight: KivoTokens.Weight.semibold)
    }

    /// A row's skeleton in [style]'s cards, laid out as the row is.
    static func skeleton(_ style: CardStyle, wide: Bool = false) -> some View {
        SkeletonRow(style: style, width: cardWidth(style, wide: wide), spacing: headingSpacing, inset: inset) {
            heading(" ")
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Self.headingSpacing) {
            header
                .foregroundStyle(palette.text)
                .accessibilityAddTraits(.isHeader)
                .padding(.horizontal, Self.inset)
            ScrollView(.horizontal, showsIndicators: false) {
                // Every card is laid out, not only those on screen, so a row of
                // mixed shapes is as tall as its tallest card. A row holds at most 20.
                HStack(alignment: .top, spacing: KivoTokens.spacing * 3) {
                    ForEach(items, id: \.id) { work in
                        let style = style ?? CardStyle.of(work)
                        WorkLink(work: work) {
                            WorkCard(work, style: style, width: Self.cardWidth(style, wide: wide))
                        }
                    }
                }
                .padding(.horizontal, Self.inset)
            }
            .scrollClipDisabled()
        }
    }

    @ViewBuilder private var header: some View {
        if let more {
            Button(action: more) {
                HStack(spacing: KivoTokens.spacing * 1.5) {
                    Self.heading(title)
                    Image(systemName: "chevron.right")
                        .font(.subheadline.weight(.bold))
                        .foregroundStyle(palette.textMuted)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
        } else {
            Self.heading(title)
        }
    }
}
