import KivoCore
import KivoUI
import SwiftUI

/// The row under a page's hero: the primary button, accent coloured, and
/// any round button [primary] puts beside it, such as Play from Beginning,
/// with a second line under it that says what it plays, then My List and
/// Mark as Watched as neutral round buttons, and any [extra], such as
/// Download. Where the primary button's label doesn't fit beside the
/// round buttons, it takes the whole width with the round buttons under it.
/// In a wide window the row keeps to the leading edge.
struct ActionRow<Primary: View, Extra: View>: View {
    let work: WorkSummary
    let page: WorkPageState
    var secondary: String?
    @ViewBuilder let primary: () -> Primary
    @ViewBuilder var extra: () -> Extra
    @Environment(\.kivoPalette) private var palette
    @Environment(\.wide) private var wide

    var body: some View {
        VStack(alignment: wide ? .leading : .center, spacing: KivoTokens.spacing * 2) {
            ActionRowLayout(spacing: KivoTokens.spacing * 3, leading: wide) {
                primary()
                roundButtons
            }
            if let secondary {
                Text(secondary)
                    .font(.footnote)
                    .foregroundStyle(palette.textMuted)
                    .multilineTextAlignment(wide ? .leading : .center)
                    .frame(maxWidth: .infinity, alignment: wide ? .leading : .center)
            }
        }
        .controlSize(.large)
        .wideWidth(KivoTokens.Container.form, wide: wide)
        .wideWidth(.infinity, wide: wide)
    }

    @ViewBuilder private var roundButtons: some View {
        UserStateButtons(work: work, page: page)
        extra()
    }
}

/// The action row's buttons: the first subview, the primary button, then
/// the round buttons. In one row the primary button takes the width the
/// round buttons leave, where its label fits; otherwise it takes the whole
/// width with the round buttons under it, centred or on the leading edge.
///
/// It measures each button once at its ideal size and reports no
/// alignment guides of its own, so the stacks around it don't ask down
/// through every button for theirs. A ViewThatFits of the two arrangements
/// measures both, over and over through those guides, and on iOS 27 the
/// row's layout as the player turns the screen runs until the watchdog
/// ends the app.
struct ActionRowLayout: Layout {
    let spacing: CGFloat
    let leading: Bool

    struct Measured {
        var primary: CGSize
        var buttons: [CGSize]

        /// The round buttons side by side.
        func buttonsWidth(spacing: CGFloat) -> CGFloat {
            buttons.map(\.width).reduce(0, +) + spacing * CGFloat(max(0, buttons.count - 1))
        }

        var buttonsHeight: CGFloat { buttons.map(\.height).max() ?? 0 }

        /// What the round buttons and the gap before them take beside the primary button.
        func beside(spacing: CGFloat) -> CGFloat {
            buttons.isEmpty ? 0 : spacing + buttonsWidth(spacing: spacing)
        }

        func fitsInRow(width: CGFloat, spacing: CGFloat) -> Bool {
            primary.width + beside(spacing: spacing) <= width
        }
    }

    func makeCache(subviews: Subviews) -> Measured {
        Measured(
            primary: subviews.first?.sizeThatFits(.unspecified) ?? .zero,
            buttons: subviews.dropFirst().map { $0.sizeThatFits(.unspecified) }
        )
    }

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout Measured) -> CGSize {
        guard let primary = subviews.first else { return .zero }
        let width = proposal.width ?? cache.primary.width + cache.beside(spacing: spacing)
        if cache.fitsInRow(width: width, spacing: spacing) {
            let primaryWidth = width - cache.beside(spacing: spacing)
            let height = primary.sizeThatFits(ProposedViewSize(width: primaryWidth, height: nil)).height
            return CGSize(width: width, height: max(height, cache.buttonsHeight))
        }
        let height = primary.sizeThatFits(ProposedViewSize(width: width, height: nil)).height
        let under = cache.buttons.isEmpty ? 0 : spacing + cache.buttonsHeight
        return CGSize(width: max(width, cache.buttonsWidth(spacing: spacing)), height: height + under)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout Measured) {
        guard let primary = subviews.first else { return }
        let buttons = Array(subviews.dropFirst())
        if cache.fitsInRow(width: bounds.width, spacing: spacing) {
            let primaryWidth = bounds.width - cache.beside(spacing: spacing)
            primary.place(at: CGPoint(x: bounds.minX, y: bounds.midY), anchor: .leading,
                          proposal: ProposedViewSize(width: primaryWidth, height: nil))
            var x = bounds.minX + primaryWidth + spacing
            for (button, size) in zip(buttons, cache.buttons) {
                button.place(at: CGPoint(x: x, y: bounds.midY), anchor: .leading, proposal: ProposedViewSize(size))
                x += size.width + spacing
            }
            return
        }
        let primaryHeight = primary.sizeThatFits(ProposedViewSize(width: bounds.width, height: nil)).height
        primary.place(at: CGPoint(x: bounds.minX, y: bounds.minY), anchor: .topLeading,
                      proposal: ProposedViewSize(width: bounds.width, height: primaryHeight))
        let rowWidth = cache.buttonsWidth(spacing: spacing)
        var x = leading ? bounds.minX : bounds.midX - rowWidth / 2
        let y = bounds.minY + primaryHeight + spacing + cache.buttonsHeight / 2
        for (button, size) in zip(buttons, cache.buttons) {
            button.place(at: CGPoint(x: x, y: y), anchor: .leading, proposal: ProposedViewSize(size))
            x += size.width + spacing
        }
    }
}

extension ActionRow where Extra == EmptyView {
    init(work: WorkSummary, page: WorkPageState, secondary: String? = nil, @ViewBuilder primary: @escaping () -> Primary) {
        self.init(work: work, page: page, secondary: secondary, primary: primary, extra: { EmptyView() })
    }
}

/// The primary button: ▶ and a short label, never truncated.
struct PlayButton: View {
    let label: String
    var identifier = "play"
    let action: () -> Void
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Button(action: action) {
            HStack(spacing: KivoTokens.spacing * 2) {
                Image(systemName: "play.fill")
                Text(label)
                    .lineLimit(1)
                    .fixedSize()
            }
            .font(.headline)
            // A list tints its buttons' text with the accent, which is this button's fill.
            .foregroundStyle(palette.onAccent)
            .frame(maxWidth: .infinity)
        }
        .accessibilityLabel(Text(label))
        .buttonStyle(.borderedProminent)
        .buttonBorderShape(.capsule)
        .accessibilityIdentifier(identifier)
    }
}

/// Play from Beginning, a round button beside Resume.
struct RestartButton: View {
    let action: () -> Void
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Button(action: action) {
            Image(systemName: Symbols.playFromBeginning)
                .roundButtonSymbol()
                .accessibilityLabel(Text("Play from beginning"))
        }
        .buttonStyle(.bordered)
        .buttonBorderShape(.circle)
        .tint(palette.text)
        .accessibilityIdentifier("play-from-beginning")
    }
}

/// My List and Mark as Watched, as on every detail page: round, neutral buttons.
struct UserStateButtons: View {
    let work: WorkSummary
    let page: WorkPageState
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    // What a tap last asked for, so a symbol animates when that answer
    // arrives and not when a read or another device changes it.
    @State private var askedOnList: Bool?
    @State private var askedPlayed: Bool?

    var body: some View {
        let myListId = page.myListId
        let onMyList = page.onMyList
        let played = page.played
        // The buttons show the state the server's events send back, so a
        // mark that fails leaves them as they were, and the app says why.
        Button {
            guard let myListId else { return }
            Haptics.success()
            askedOnList = !onMyList
            server.session.library.marks.setOnList(listId: myListId, workId: work.id, on: !onMyList)
        } label: {
            Image(systemName: Symbols.myList(onMyList))
                .roundButtonSymbol()
                .symbolReplace(animated: askedOnList == onMyList)
                .accessibilityLabel(onMyList ? Text("Remove from Watchlist") : Text("Add to Watchlist"))
        }
        .buttonStyle(.bordered)
        .buttonBorderShape(.circle)
        .tint(onMyList ? palette.accent : palette.text)
        .disabled(myListId == nil)
        .accessibilityIdentifier("my-list")
        Button {
            Haptics.success()
            askedPlayed = !played
            server.session.library.marks.setPlayed(workId: work.id, played: !played)
        } label: {
            Image(systemName: Symbols.watched(played))
                .roundButtonSymbol()
                .symbolReplace(animated: askedPlayed == played)
                .accessibilityLabel(Text(Words.markPlayed(!played, kind: work.kind)))
        }
        .buttonStyle(.bordered)
        .buttonBorderShape(.circle)
        .tint(palette.text)
        .accessibilityIdentifier("watched")
    }
}

/// A round button's symbol, at the size every one has, which grows with the text size.
private struct RoundButtonSymbol: ViewModifier {
    @ScaledMetric(relativeTo: .headline) private var side = 24.0

    func body(content: Content) -> some View {
        content.font(.headline).frame(width: side, height: side)
    }
}

extension View {
    /// A round button's symbol, at the size every one has.
    func roundButtonSymbol() -> some View {
        modifier(RoundButtonSymbol())
    }

    /// Swaps a symbol with the replace effect when [animated], else at once.
    func symbolReplace(animated: Bool) -> some View {
        contentTransition(animated ? .symbolEffect(.replace) : .identity)
    }

    /// A text button's label, tall enough to tap: 44 points at least.
    func tapTarget() -> some View {
        frame(minHeight: 44).contentShape(Rectangle())
    }

    /// The list row of a page's actions, under its hero.
    func actionRowInsets() -> some View {
        detailRow(EdgeInsets(top: 4, leading: 20, bottom: 12, trailing: 20))
    }
}

/// An ``ActionRow``'s skeleton: the primary button's capsule and
/// [roundButtons] circles, each the size of the button it stands for.
struct ActionRowSkeleton: View {
    let roundButtons: Int

    var body: some View {
        HStack(spacing: KivoTokens.spacing * 3) {
            PlayButton(label: String(localized: "Play")) {}
                .skeleton(Capsule())
            ForEach(0..<roundButtons, id: \.self) { _ in
                Button {} label: { Image(systemName: "plus").roundButtonSymbol() }
                    .buttonStyle(.bordered)
                    .buttonBorderShape(.circle)
                    .skeleton(Circle())
            }
        }
        .controlSize(.large)
    }
}
