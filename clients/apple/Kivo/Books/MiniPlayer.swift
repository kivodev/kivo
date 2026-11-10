import KivoCore
import KivoUI
import SwiftUI

/// The mini player while a book is loaded: its cover, title
/// and "Chapter 12 · 4 hr 21 min left", Back 10 seconds, Play or Pause and
/// Forward 30 seconds. A tap or a swipe up opens Now Playing. On iOS 26 it's
/// the tab bar's accessory, and with the tab bar minimised only the cover,
/// title and Play or Pause; earlier, a bar above the tab bar.
struct MiniPlayer: View {
    let book: BookPlayer
    var compact = false

    var body: some View {
        HStack(spacing: KivoTokens.spacing * 3) {
            Button {
                book.expanded = true
            } label: {
                HStack(spacing: KivoTokens.spacing * 3) {
                    BookCover(work: book.work, size: compact ? 28 : 40)
                    VStack(alignment: .leading, spacing: 1) {
                        Text(book.work?.title ?? "")
                            .font(.subheadline.weight(.semibold))
                            .lineLimit(1)
                        if !compact, let line = book.miniLine {
                            Text(line)
                                .font(.caption)
                                .foregroundStyle(.secondary)
                                .lineLimit(1)
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityHint(Text("Opens Now Playing"))
            .accessibilityIdentifier("miniOpen")
            if !compact {
                button("gobackward.10", label: String(localized: "Back 10 seconds"), action: book.skipBack)
                    .accessibilityIdentifier("miniBack")
            }
            PlayPauseButton(book: book, size: compact ? 17 : 22)
                .frame(width: 36, height: 36)
                .accessibilityIdentifier("miniPlay")
            if !compact {
                button("goforward.30", label: String(localized: "Forward 30 seconds"), action: book.skipForward)
                    .accessibilityIdentifier("miniForward")
            }
        }
        .padding(.leading, compact ? KivoTokens.spacing * 2 : KivoTokens.spacing * 3)
        .padding(.trailing, KivoTokens.spacing * 2)
        .contentShape(Rectangle())
        .simultaneousGesture(DragGesture(minimumDistance: 12).onEnded { value in
            if value.translation.height < -24 { book.expanded = true }
        })
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("miniPlayer")
    }

    private func button(_ symbol: String, label: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: symbol)
                .font(.system(size: 18, weight: .medium))
                .frame(width: 36, height: 36)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(Text(label))
    }
}

/// Play or Pause, a spinner while the book opens.
struct PlayPauseButton: View {
    let book: BookPlayer
    var size: CGFloat

    var body: some View {
        Button(action: book.togglePlay) {
            ZStack {
                if book.loading {
                    ProgressView()
                } else {
                    Image(systemName: book.playing ? "pause.fill" : "play.fill")
                        .font(.system(size: size, weight: .semibold))
                        .contentTransition(.symbolEffect(.replace))
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(book.playing ? Text("Pause") : Text("Play"))
    }
}

/// The mini player as the tab bar's accessory, which iOS 26 draws in glass
/// above the tabs and shrinks beside them when the tab bar minimises.
@available(iOS 26.1, *)
struct MiniPlayerAccessory: View {
    let book: BookPlayer
    @Environment(\.tabViewBottomAccessoryPlacement) private var placement
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        // The accessory takes the system's appearance rather than the app's theme: the theme's colours, explicitly.
        MiniPlayer(book: book, compact: placement == .inline)
            .foregroundStyle(palette.text, palette.textMuted)
            .environment(\.colorScheme, palette.scheme)
    }
}

/// The cast's mini controller as the tab bar's accessory, in the book mini player's place.
@available(iOS 26.1, *)
struct CastMiniAccessory: View {
    let cast: CastModel
    let open: () -> Void
    @Environment(\.tabViewBottomAccessoryPlacement) private var placement
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        CastMiniController(cast: cast, open: open, compact: placement == .inline)
            .foregroundStyle(palette.text, palette.textMuted)
            .environment(\.colorScheme, palette.scheme)
    }
}

/// Before iOS 26.1, the mini player is a bar pinned above the tab bar, with
/// the book's progress along its top edge, on each tab's pages; while
/// casting, the cast's mini controller is.
struct MiniPlayerBar: ViewModifier {
    let book: BookPlayer
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette

    func body(content: Content) -> some View {
        if #available(iOS 26.1, *) {
            content
        } else {
            content.safeAreaInset(edge: .bottom, spacing: 0) {
                if server.cast.active {
                    CastMiniController(cast: server.cast, open: { server.player.shown = true })
                        .frame(height: 60)
                        .background(.bar)
                        .overlay(alignment: .bottom) { Divider() }
                } else if book.loaded {
                    VStack(spacing: 0) {
                        GeometryReader { geometry in
                            palette.accent
                                .frame(width: geometry.size.width * book.fraction)
                        }
                        .frame(height: 2)
                        MiniPlayer(book: book)
                            .frame(height: 60)
                    }
                    .background(.bar)
                    .overlay(alignment: .bottom) { Divider() }
                }
            }
        }
    }
}
