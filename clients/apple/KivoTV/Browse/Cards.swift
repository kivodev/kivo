import KivoCore
import KivoUI
import SwiftUI

/// The TV's card sizes and gaps, in points on the 1080p canvas: the Android
/// TV app's, at two points to its dp.
enum TVLayout {
    /// The gap between cards, which leaves a focused card's lift clear of its neighbours.
    static let cardGap: CGFloat = 56
    /// The gap between rows.
    static let rowGap: CGFloat = 42
    /// A row's title's gap to its cards.
    static let titleGap: CGFloat = 20
    /// The gap between a grid's lines.
    static let lineGap: CGFloat = 40
    /// An episode card's width in a season's row.
    static let episodeWidth: CGFloat = 362
    /// A person's card's width, their round photo's too.
    static let personWidth: CGFloat = 224

    /// A card's width in a row: posters and squares, or landscape cards.
    static func rowWidth(_ style: CardStyle) -> CGFloat {
        style == .landscape ? 500 : 220
    }

    /// A card's width in a grid, six posters or three landscape cards
    /// across, which leaves room for the letters beside a library's.
    static func gridWidth(_ style: CardStyle) -> CGFloat {
        style == .landscape ? 472 : 208
    }

    /// Cards across a grid.
    static func gridColumns(_ style: CardStyle) -> Int {
        style == .landscape ? 3 : 6
    }
}

/// A work's card as a focusable lockup: its artwork lifts with focus as the
/// system's do, and Select opens its page. Holding Select opens its menu
/// (WorkMenu).
struct WorkLockup: View {
    let work: WorkSummary
    let style: CardStyle
    let width: CGFloat
    var caption: CardCaption = .twoLines
    @Environment(\.openWork) private var openWork

    var body: some View {
        Button {
            openWork(WorkRoute(work))
        } label: {
            WorkCard(work, style: style, width: width, caption: caption)
        }
        .buttonStyle(.borderless)
        .contextMenu { WorkMenu(work: work) }
    }
}

/// A titled row of cards that scrolls sideways under focus. A list's row
/// ends in See all, which opens the list.
struct CardRow<Cards: View>: View {
    let title: String
    @ViewBuilder let cards: () -> Cards
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        VStack(alignment: .leading, spacing: TVLayout.titleGap) {
            Text(title)
                .kivoStyle(KivoTokens.TextSize.lg, weight: KivoTokens.Weight.semibold)
                .foregroundStyle(palette.text)
            ScrollView(.horizontal) {
                LazyHStack(alignment: .top, spacing: TVLayout.cardGap) {
                    cards()
                }
            }
            .scrollClipDisabled()
        }
        .focusSection()
    }
}

/// A row of works' cards: each work's own shape, or [style] for every one.
struct WorkRow: View {
    let title: String
    let works: [WorkSummary]
    var style: CardStyle?
    /// Opens all of what the row starts, as a list's See all.
    var seeAll: (() -> Void)?

    var body: some View {
        if !works.isEmpty {
            CardRow(title: title) {
                ForEach(works, id: \.id) { work in
                    let style = style ?? CardStyle.of(work)
                    WorkLockup(work: work, style: style, width: TVLayout.rowWidth(style))
                }
                if let seeAll {
                    SeeAllCard(style: style ?? .poster, action: seeAll)
                }
            }
        }
    }
}

/// See all, at a row's end: a card of the row's shape that opens what the row starts.
struct SeeAllCard: View {
    let style: CardStyle
    let action: () -> Void
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        let width = TVLayout.rowWidth(style)
        Button(action: action) {
            VStack(spacing: KivoTokens.spacing * 2) {
                Image(systemName: "chevron.right")
                    .font(.title3.weight(.semibold))
                Text("See all")
                    .kivoStyle(KivoTokens.TextSize.sm, weight: KivoTokens.Weight.medium)
            }
            .foregroundStyle(palette.textMuted)
            .frame(width: width, height: width / style.aspect)
            .background(palette.surfaceRaised, in: RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
            .cardLift()
        }
        .buttonStyle(.borderless)
    }
}

/// A row's skeleton, laid out as the row is, while a screen first loads.
struct SkeletonCardRow: View {
    let style: CardStyle

    var body: some View {
        SkeletonRow(style: style, width: TVLayout.rowWidth(style), spacing: TVLayout.titleGap, inset: 0, gap: TVLayout.cardGap) {
            Text(verbatim: " ").kivoStyle(KivoTokens.TextSize.lg, weight: KivoTokens.Weight.semibold)
        }
    }
}
