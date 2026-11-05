import KivoCore
import KivoUI
import SwiftUI

/// The frame of every detail page: the backdrop or still full
/// bleed to the top edge, under the status bar and the floating back,
/// Cast and ⋯ buttons (no Cast on a book's), darkening into the page below it, with the title, its logo
/// where there is one, and the details line over its foot. The rest of the
/// page is a list, so episode rows can be swiped. The art follows the
/// scroll and stretches when pulled down; the title comes into the top bar
/// once the hero has scrolled away. Without a work, it's the page's
/// skeleton: the art's place and the hero's lines, over [rows]' skeletons.
/// A read that failed says why under the hero, over what's known.
/// In a wide window the art is shorter for its width, and the hero and the
/// rows under it line up on the leading edge, as a desktop page's do.
struct DetailPage<Rows: View, Actions: View>: View {
    let work: WorkDetail?
    let art: ImageRef?
    /// A line above the title, such as an episode's series.
    var eyebrow: (label: String, open: () -> Void)?
    let details: [String]
    @ViewBuilder let rows: () -> Rows
    /// The ⋯ menu's contents.
    @ViewBuilder let actions: () -> Actions
    @Environment(\.kivoPalette) private var palette
    @Environment(\.wide) private var wide
    @Environment(ServerModel.self) private var server
    @Environment(\.detailLoad) private var load
    @State private var scroll: CGFloat = 0

    var body: some View {
        GeometryReader { geometry in
            let top = geometry.safeAreaInsets.top
            // The art shows at 16:9 below the top bar, and runs up under it.
            let artHeight = wide
                ? min(geometry.size.width * 9 / 16, geometry.size.height * 0.55)
                : min(geometry.size.width * 9 / 16 + 56, geometry.size.height * 0.62)
            ZStack(alignment: .top) {
                backdrop(height: artHeight + top, overscroll: max(0, -scroll))
                    .offset(y: -max(0, scroll))
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                    .ignoresSafeArea()
                List {
                    hero
                        .frame(maxWidth: .infinity, minHeight: artHeight, alignment: wide ? .bottomLeading : .bottom)
                        .detailRow(EdgeInsets(top: 0, leading: 20, bottom: 12, trailing: 20))
                    if let problem = load.problem {
                        InlineFailure(title: String(localized: "Couldn't load this page."), problem: problem, retry: load.retry)
                    }
                    rows()
                }
                .listStyle(.plain)
                .scrollContentBackground(.hidden)
                .environment(\.defaultMinListRowHeight, 0)
                .onScrollGeometryChange(for: CGFloat.self) { geometry in
                    geometry.contentOffset.y + geometry.contentInsets.top
                } action: { _, offset in
                    scroll = offset
                }
            }
            .toolbar {
                ToolbarItem(placement: .principal) {
                    Text(work?.labelledTitle ?? "")
                        .font(.headline)
                        .lineLimit(1)
                        .opacity(scroll > artHeight - 24 ? 1 : 0)
                        .animation(.easeInOut(duration: 0.2), value: scroll > artHeight - 24)
                }
                ToolbarItemGroup(placement: .topBarTrailing) {
                    if let work, work.book == nil {
                        CastButton(cast: server.cast)
                            .tint(palette.text)
                    }
                    Menu {
                        actions()
                    } label: {
                        Label(String(localized: "More"), systemImage: "ellipsis")
                    }
                    .tint(palette.text)
                    .disabled(work == nil)
                    .accessibilityIdentifier("more")
                }
            }
        }
        .navigationTitle(work?.labelledTitle ?? "")
        .toolbarTitleDisplayMode(.inline)
        .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
    }

    private func backdrop(height: CGFloat, overscroll: CGFloat) -> some View {
        ZStack {
            if work == nil {
                SkeletonBlock(radius: 0)
            } else {
                ArtworkImage(art, title: "", pointWidth: wide ? 1024 : 430)
            }
        }
            .frame(maxWidth: .infinity)
            .frame(height: height + overscroll)
            .overlay(alignment: .top) {
                // Keeps the status bar and the floating buttons readable on bright art.
                LinearGradient(colors: [.black.opacity(0.45), .clear], startPoint: .top, endPoint: .bottom)
                    .frame(height: 140)
            }
            .overlay(alignment: .bottom) {
                LinearGradient(
                    stops: [
                        .init(color: palette.bg.opacity(0), location: 0),
                        .init(color: palette.bg.opacity(0.75), location: 0.55),
                        .init(color: palette.bg, location: 1),
                    ],
                    startPoint: .top,
                    endPoint: .bottom
                )
                .frame(height: height * 0.55)
            }
            .clipped()
    }

    /// The work's logo. An episode is headed by its own title, as its logo
    /// is its series', and a movie its folder labels by its labelled title,
    /// as its logo names the film alone.
    private var logo: ImageRef? {
        work?.episode == nil && work?.movie?.edition == nil ? work?.images.logo : nil
    }

    @ViewBuilder private var hero: some View {
        if let work {
            hero(work)
        } else {
            heroSkeleton
        }
    }

    /// The hero's lines in their styles: the eyebrow when there'll be one, the title and the details.
    private var heroSkeleton: some View {
        VStack(alignment: wide ? .leading : .center, spacing: KivoTokens.spacing * 2) {
            if eyebrow != nil {
                Text(verbatim: " ").font(.subheadline.weight(.semibold)).skeletonLine(width: 120, alignment: .center)
            }
            Text(verbatim: " ").font(.system(.title, design: .default, weight: .bold)).skeletonLine(width: 220, alignment: .center)
            Text(verbatim: " ").font(.footnote).skeletonLine(width: 200, alignment: .center)
        }
        .frame(maxWidth: .infinity, alignment: wide ? .leading : .center)
    }

    private func hero(_ work: WorkDetail) -> some View {
        VStack(alignment: wide ? .leading : .center, spacing: KivoTokens.spacing * 2) {
            if let eyebrow {
                Button(action: eyebrow.open) {
                    Text(eyebrow.label)
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(palette.text.opacity(0.85))
                        .tapTarget()
                }
                .buttonStyle(.plain)
            }
            LogoImage(logo, pointWidth: 260) {
                Text(work.labelledTitle)
                    .font(.system(wide ? .largeTitle : .title, design: .default, weight: .bold))
                    .foregroundStyle(palette.text)
                    .multilineTextAlignment(wide ? .leading : .center)
                    .accessibilityAddTraits(.isHeader)
            }
            .frame(maxWidth: logo == nil ? nil : 260, maxHeight: logo == nil ? nil : 84, alignment: wide ? .leading : .center)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Text(work.labelledTitle))
            .accessibilityAddTraits(.isHeader)
            if !details.isEmpty {
                Text(details.joined(separator: " · "))
                    .font(wide ? .subheadline : .footnote)
                    .foregroundStyle(palette.textMuted)
                    .multilineTextAlignment(wide ? .leading : .center)
            }
        }
        .frame(maxWidth: wide ? KivoTokens.Container.prose : .infinity, alignment: wide ? .leading : .center)
    }
}

extension View {
    /// A detail page's list row: no separator or background, with its own insets.
    func detailRow(_ insets: EdgeInsets = EdgeInsets(top: 8, leading: 20, bottom: 8, trailing: 20)) -> some View {
        listRowInsets(insets)
            .listRowSeparator(.hidden)
            .listRowBackground(Color.clear)
    }

    /// In a wide window, at most [width] wide on the leading edge; in a
    /// narrow one, as it is, with no frame for the stacks around it to
    /// measure through.
    @ViewBuilder func wideWidth(_ width: CGFloat, wide: Bool) -> some View {
        if wide {
            frame(maxWidth: width, alignment: .leading)
        } else {
            self
        }
    }
}
