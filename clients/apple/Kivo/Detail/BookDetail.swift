import KivoCore
import KivoUI
import SwiftUI

/// A book's page: Play, or Resume with the time left, which starts the
/// book player where the user left off in any app or browser, and Play
/// from beginning beside it; My List, Mark as Finished and Download; the
/// overview, its authors and narrators, and its chapters, each starting
/// the book there. It streams, or plays its download when one is ready,
/// which the badge under the actions names.
struct BookDetail: View {
    /// What shows of the book: its page's summary once read.
    let work: WorkSummary
    let page: WorkPageState
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette

    private var chapters: BookChapters? {
        page.work?.versions.first.map { BookChapters(version: $0) }
    }

    var body: some View {
        DetailPage(work: work, art: work.images.backdrop ?? work.images.poster, details: details) {
            actions
                .actionRowInsets()
            Overview(text: page.work?.overview)
            CastRow(credits: page.work?.credits ?? [])
            if work.available, let chapters, chapters.count > 1 {
                SectionHeading(title: String(localized: "Chapters"))
                    .detailRow(EdgeInsets(top: 20, leading: 20, bottom: 4, trailing: 20))
                ForEach(Array(chapters.list.enumerated()), id: \.offset) { index, chapter in
                    chapterRow(index: index, chapter: chapter)
                }
            }
            Color.clear.frame(height: KivoTokens.spacing * 6).detailRow()
        } actions: {
            Section {
                UserStateMenuItems(work: work)
                GoToLibraryItem(libraryId: work.libraryId)
            }
        }
    }

    /// "2021 · 16h 10m · Science Fiction".
    private var details: [String] {
        var parts: [String] = []
        if let year = work.year { parts.append(String(year.intValue)) }
        if let ms = work.durationMs?.int64Value { parts.append(Display.app.runtime(ms: ms)) }
        if let genres = page.work?.genres, !genres.isEmpty { parts.append(genres.prefix(2).joined(separator: ", ")) }
        return parts
    }

    @ViewBuilder private var actions: some View {
        if work.available {
            VStack(spacing: KivoTokens.spacing * 3) {
                ActionRow(work: work, page: page, secondary: work.timeLeft) {
                    PlayButton(label: work.resumeMs != nil ? String(localized: "Resume") : String(localized: "Play")) {
                        server.listen(work.id)
                    }
                    if work.resumeMs != nil {
                        RestartButton { server.listen(work.id, at: 0) }
                    }
                } extra: {
                    DownloadButton(work: work, choice: PlayChoice(workId: work.id))
                }
                DownloadBadge(workId: work.id)
            }
        } else {
            VStack(spacing: KivoTokens.spacing * 3) {
                Text("This file is missing")
                    .font(.headline)
                    .foregroundStyle(palette.danger)
                HStack(spacing: KivoTokens.spacing * 3) { UserStateButtons(work: work, page: page) }
                    .controlSize(.large)
            }
            .frame(maxWidth: .infinity)
        }
    }

    private func chapterRow(index: Int, chapter: Chapter) -> some View {
        let current = server.book.work?.id == work.id && server.book.chapterIndex.map(Int.init) == index
        return Button {
            server.listen(work.id, at: Int64(chapter.startMs))
        } label: {
            HStack(spacing: KivoTokens.spacing * 3) {
                Text(BookWords.chapter(title: chapter.title, index: index))
                    .font(.subheadline.weight(current ? .semibold : .regular))
                    .foregroundStyle(current ? palette.accent : palette.text)
                    .lineLimit(2)
                    .multilineTextAlignment(.leading)
                Spacer(minLength: KivoTokens.spacing * 2)
                Text(Display.app.clock(ms: Int64(chapter.startMs)))
                    .font(.subheadline.monospacedDigit())
                    .foregroundStyle(palette.textMuted)
            }
            .padding(.vertical, KivoTokens.spacing * 1.5)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("bookChapter")
        .detailRow(EdgeInsets(top: 0, leading: 20, bottom: 0, trailing: 20))
    }
}
