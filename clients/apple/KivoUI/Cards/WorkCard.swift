import KivoCore
import SwiftUI

/// A work's card: the artwork with its corner marks and
/// progress, then the title and a second line, or one line in a grid.
/// [mark] is a short mark at the artwork's bottom trailing corner, such as
/// a trailer's running time.
public struct WorkCard: View {
    private let work: WorkSummary
    private let style: CardStyle
    private let width: CGFloat
    private let caption: CardCaption
    private let mark: String?
    @Environment(\.kivoPalette) private var palette

    public init(_ work: WorkSummary, style: CardStyle? = nil, width: CGFloat, caption: CardCaption = .twoLines, mark: String? = nil) {
        self.work = work
        self.style = style ?? CardStyle.of(work)
        self.width = width
        self.caption = caption
        self.mark = mark
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing) {
            ZStack {
                ArtworkImage(image, title: work.title, pointWidth: width)
                marks
            }
            .frame(width: width, height: width / style.aspect)
            .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
            // The shadow is the shape's behind the artwork, which draws
            // once, rather than one worked out from the artwork itself.
            .background {
                RoundedRectangle(cornerRadius: KivoTokens.Radius.md)
                    .fill(palette.placeholder)
                    .kivoShadow(palette.cardShadow)
            }
            .cardLift()

            switch caption {
            case .twoLines:
                VStack(alignment: .leading, spacing: 0) {
                    Text(heading)
                        .kivoStyle(KivoTokens.TextSize.sm, weight: KivoTokens.Weight.medium)
                        .foregroundStyle(palette.text)
                        .lineLimit(1)
                    // Kept when there's no second line, so cards in a row are one height.
                    Text(secondLine ?? "")
                        .kivoStyle(KivoTokens.TextSize.xs)
                        .foregroundStyle(palette.textMuted)
                        .lineLimit(1, reservesSpace: true)
                }
                .frame(width: width, alignment: .leading)
            case .oneLine:
                oneLine
                    .frame(width: width, alignment: .leading)
            }
        }
        // One element with the card's own frame: artwork filling its clip can be wider than the card.
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text(accessibilityText))
    }

    /// What VoiceOver reads: the title and the second line, and the marks on the artwork.
    private var accessibilityText: String {
        var parts = [heading]
        if let line = caption == .oneLine ? work.gridDetail : secondLine { parts.append(line) }
        if let episode = work.episode { parts.append(Display.app.episodeCode(episode: episode)) }
        if let mark { parts.append(mark) }
        if let badge = work.matchBadge { parts.append(badge) }
        if let series = work.series {
            parts.append(series.unplayedCount > 0 ? String(localized: "\(series.unplayedCount) unwatched") : String(localized: "Watched"))
        } else if work.userState?.played == true {
            parts.append(String(localized: "Watched"))
        }
        if let progress = work.progress { parts.append(String(localized: "\(Int(progress * 100))% watched")) }
        return parts.joined(separator: ", ")
    }

    /// The title and, when it fits beside it, the year.
    private var oneLine: some View {
        let title = Text(work.labelledTitle)
            .kivoStyle(KivoTokens.TextSize.xs, weight: KivoTokens.Weight.medium)
            .foregroundStyle(palette.text)
            .lineLimit(1)
        return ViewThatFits(in: .horizontal) {
            if let detail = work.gridDetail {
                HStack(spacing: KivoTokens.spacing * 1.5) {
                    title.fixedSize()
                    Text(detail)
                        .kivoStyle(KivoTokens.TextSize.xs)
                        .foregroundStyle(palette.textMuted)
                        .lineLimit(1)
                        .fixedSize()
                }
            }
            title
        }
    }

    private var image: ImageRef? { style.artwork(of: work) }

    /// An episode's card is headed by its show, its code on the artwork.
    private var heading: String {
        work.episode?.seriesTitle ?? work.labelledTitle
    }

    private var secondLine: String? {
        guard work.episode != nil else { return work.caption }
        return [work.title, work.timeLeft].compactMap { $0 }.joined(separator: " · ")
    }

    private var marks: some View {
        ZStack {
            VStack {
                HStack {
                    Spacer()
                    if let corner = cornerMark { corner.padding(KivoTokens.spacing * 1.5) }
                }
                Spacer()
                HStack {
                    // The match badge moves right on a landscape card or beside an episode's code.
                    let right = style == .landscape || work.episode != nil
                    if let episode = work.episode {
                        Badge(text: Display.app.episodeCode(episode: episode))
                    }
                    if !right, let badge = work.matchBadge { Badge(text: badge) }
                    Spacer()
                    if right, let badge = work.matchBadge { Badge(text: badge) }
                    if let mark { Badge(text: mark) }
                }
                .padding(KivoTokens.spacing * 1.5)
            }
            if let progress = work.progress {
                VStack {
                    Spacer()
                    ProgressBar(value: progress)
                }
            }
        }
    }

    /// Played shows a check; a series its unwatched count, a check once all are played.
    private var cornerMark: AnyView? {
        if let series = work.series {
            if series.unplayedCount > 0 { return AnyView(Badge(text: "\(series.unplayedCount)")) }
            return AnyView(PlayedCheck())
        }
        if let season = work.season {
            if season.unplayedCount > 0 { return AnyView(Badge(text: "\(season.unplayedCount)")) }
            return AnyView(PlayedCheck())
        }
        return work.userState?.played == true ? AnyView(PlayedCheck()) : nil
    }
}
