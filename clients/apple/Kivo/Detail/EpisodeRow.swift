import KivoCore
import KivoUI
import SwiftUI

/// An episode in a season's list, as a compact width shows them: its still
/// with the progress bar, "1. Title", the runtime or time left, and a check
/// once watched, then up to three lines of its synopsis.
/// Tapping opens its page; swiping marks it watched or unwatched, and a
/// long press has Play and the rest.
struct EpisodeRow: View {
    let episode: WorkSummary
    var current = false
    @Environment(ServerModel.self) private var server
    @Environment(\.openWork) private var openWork
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        let played = episode.userState?.played == true
        Button {
            openWork(WorkRoute(episode))
        } label: {
            VStack(alignment: .leading, spacing: KivoTokens.spacing * 2) {
                HStack(spacing: KivoTokens.spacing * 3) {
                    ZStack(alignment: .bottom) {
                        ArtworkImage(episode.images.still ?? episode.images.backdrop, title: episode.title, pointWidth: 128)
                        if let progress = episode.progress {
                            ProgressBar(value: progress)
                        }
                    }
                    .frame(width: 128, height: 72)
                    .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.md, style: .continuous))
                    VStack(alignment: .leading, spacing: 3) {
                        Text(heading)
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(current ? palette.accent : palette.text)
                            .lineLimit(2)
                            .multilineTextAlignment(.leading)
                        HStack(spacing: KivoTokens.spacing * 1.5) {
                            if let line = episode.timeLeft ?? episode.durationMs.map({ Display.app.runtime(ms: $0.int64Value) }) {
                                Text(line)
                            }
                            if let badge = episode.matchBadge { Text(badge) }
                        }
                        .font(.footnote)
                        .foregroundStyle(palette.textMuted)
                    }
                    Spacer(minLength: 0)
                    if played {
                        Image(systemName: "checkmark.circle.fill")
                            .font(.body)
                            .foregroundStyle(palette.textMuted)
                            .accessibilityLabel(Text("Watched"))
                    }
                }
                if let synopsis = episode.synopsis {
                    Text(verbatim: synopsis)
                        .font(.footnote)
                        .foregroundStyle(palette.textMuted)
                        .lineLimit(3)
                        .multilineTextAlignment(.leading)
                }
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("episode")
        .contextMenu { WorkActions(work: episode, showsSeries: false) }
        .swipeActions(edge: .leading, allowsFullSwipe: true) {
            Button {
                Haptics.success()
                server.session.library.marks.setPlayed(workId: episode.id, played: !played)
            } label: {
                Label(played ? String(localized: "Unwatched") : String(localized: "Watched"), systemImage: played ? "eye.slash" : "checkmark.circle")
            }
            .tint(played ? .gray : palette.accent)
        }
        .detailRow(EdgeInsets(top: 6, leading: 20, bottom: 6, trailing: 20))
    }

    private var heading: String { episode.episodeHeading }
}

/// A season's episodes as a row of cards that scrolls sideways, as a
/// regular width shows them: each its still with the progress bar and a
/// check once watched, then "1. Title" and the runtime or time left before
/// two lines of its synopsis. It opens with [focus] in view: Next Up on a series' page, the open episode
/// on an episode's. Tapping opens the episode; a long press has Play, Mark
/// as Watched and the rest.
struct EpisodeShelf: View {
    let episodes: [WorkSummary]
    var current: String?
    var focus: String?
    @Environment(\.wide) private var wide

    var body: some View {
        let width = CardRow.cardWidth(.landscape, wide: wide)
        // A season with no synopses keeps its cards a line shorter.
        let lines = episodes.contains { $0.synopsis != nil } ? 2 : 1
        ScrollViewReader { proxy in
            ScrollView(.horizontal, showsIndicators: false) {
                LazyHStack(alignment: .top, spacing: KivoTokens.spacing * 3) {
                    ForEach(episodes, id: \.id) { episode in
                        EpisodeCard(episode: episode, width: width, lines: lines, current: episode.id == current)
                    }
                }
                .padding(.horizontal, CardRow.inset)
            }
            .scrollClipDisabled()
            // A new season's episodes can come after the shelf appears.
            .onChange(of: episodes.contains { $0.id == focus }, initial: true) { _, shown in
                if shown, let focus { proxy.scrollTo(focus, anchor: .center) }
            }
        }
        .shelfInsets()
    }
}

/// The shelf's cards while a season's episodes are read.
struct EpisodeShelfSkeleton: View {
    @Environment(\.wide) private var wide

    var body: some View {
        SkeletonRow(style: .landscape, width: CardRow.cardWidth(.landscape, wide: wide), spacing: 0, inset: CardRow.inset) {
            EmptyView()
        }
        .shelfInsets()
        .accessibilityHidden(true)
    }
}

private struct EpisodeCard: View {
    let episode: WorkSummary
    let width: CGFloat
    let lines: Int
    let current: Bool
    @Environment(\.openWork) private var openWork
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Button {
            openWork(WorkRoute(episode))
        } label: {
            VStack(alignment: .leading, spacing: KivoTokens.spacing * 2) {
                ZStack(alignment: .bottom) {
                    ArtworkImage(episode.images.still ?? episode.images.backdrop, title: episode.title, pointWidth: width)
                    if let progress = episode.progress {
                        ProgressBar(value: progress)
                    }
                }
                .frame(width: width, height: width / CardStyle.landscape.aspect)
                .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
                .background {
                    RoundedRectangle(cornerRadius: KivoTokens.Radius.md)
                        .fill(palette.placeholder)
                        .kivoShadow(palette.cardShadow)
                }
                .overlay(alignment: .topTrailing) {
                    if played { PlayedCheck().padding(KivoTokens.spacing * 1.5) }
                }
                VStack(alignment: .leading, spacing: KivoTokens.spacing) {
                    Text(episode.episodeHeading)
                        .kivoStyle(KivoTokens.TextSize.sm, weight: KivoTokens.Weight.semibold)
                        .foregroundStyle(current ? palette.accent : palette.text)
                        .lineLimit(1)
                    // Kept at the shelf's lines when shorter, so the cards are one height.
                    detail
                        .kivoStyle(KivoTokens.TextSize.xs)
                        .foregroundStyle(palette.textMuted)
                        .lineLimit(lines, reservesSpace: true)
                        .multilineTextAlignment(.leading)
                }
                .frame(width: width, alignment: .leading)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .hoverEffect(.highlight)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text(accessibilityText))
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("episode")
        .contextMenu { WorkActions(work: episode, showsSeries: false) }
    }

    private var played: Bool { episode.userState?.played == true }

    private var secondLine: String? {
        let parts = [episode.timeLeft ?? episode.durationMs.map { Display.app.runtime(ms: $0.int64Value) }, episode.matchBadge]
        let line = parts.compactMap { $0 }.joined(separator: " · ")
        return line.isEmpty ? nil : line
    }

    /// The runtime, or the time left in the accent, then the synopsis.
    private var detail: Text {
        let lead = secondLine.map {
            Text(verbatim: $0)
                .fontWeight(KivoTokens.Weight.medium)
                .foregroundStyle(episode.timeLeft != nil ? palette.accent : palette.text)
        }
        switch (lead, episode.synopsis) {
        case let (lead?, synopsis?): return Text("\(lead) · \(synopsis)")
        case let (lead?, nil): return lead
        case let (nil, synopsis?): return Text(verbatim: synopsis)
        case (nil, nil): return Text(verbatim: "")
        }
    }

    private var accessibilityText: String {
        var parts = [episode.episodeHeading]
        if let secondLine { parts.append(secondLine) }
        if let synopsis = episode.synopsis { parts.append(synopsis) }
        if played { parts.append(String(localized: "Watched")) }
        if let progress = episode.progress { parts.append(String(localized: "\(Int(progress * 100))% watched")) }
        return parts.joined(separator: ", ")
    }
}

private extension View {
    /// The list row of an episode shelf, under the season's heading.
    func shelfInsets() -> some View {
        detailRow(EdgeInsets(top: 8, leading: 0, bottom: 8, trailing: 0))
    }
}

private extension WorkSummary {
    /// An episode's synopsis on one line, without markup; nil when it has none.
    var synopsis: String? {
        guard let overview = episode?.overview else { return nil }
        let text = overview
            .replacingOccurrences(of: "<[^>]+>", with: " ", options: .regularExpression)
            .split(whereSeparator: \.isWhitespace)
            .joined(separator: " ")
        return text.isEmpty ? nil : text
    }

    /// "4. Reload", or "1–2. Pilot / The Big Empty" for a multi-episode file.
    var episodeHeading: String {
        guard let info = episode else { return title }
        let end = info.endNumber.map { "–\($0.intValue)" } ?? ""
        return "\(info.number)\(end). \(title)"
    }
}

/// Episode rows' skeletons while a season's episodes are read: as many as
/// the season has, where that's known, else a few.
struct EpisodeRowSkeletons: View {
    let count: Int?

    var body: some View {
        ForEach(0..<min(count ?? 3, 12), id: \.self) { _ in
            HStack(spacing: KivoTokens.spacing * 3) {
                SkeletonBlock(width: 128, height: 72)
                VStack(alignment: .leading, spacing: 3) {
                    Text(verbatim: " ").font(.subheadline.weight(.semibold)).skeletonLine(width: 160)
                    Text(verbatim: " ").font(.footnote).skeletonLine(width: 60)
                }
                Spacer(minLength: 0)
            }
            .detailRow(EdgeInsets(top: 6, leading: 20, bottom: 6, trailing: 20))
            .accessibilityHidden(true)
        }
    }
}
