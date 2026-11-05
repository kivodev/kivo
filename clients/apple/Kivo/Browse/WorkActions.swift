import KivoCore
import SwiftUI

/// The actions on a work, for its long press menu and an episode row's;
/// a movie, episode or video also downloads at the download quality
/// setting, or removes its download.
struct WorkActions: View {
    let work: WorkSummary
    var showsSeries = true
    @Environment(ServerModel.self) private var server
    @Environment(\.openWork) private var openWork
    @Environment(\.currentLibraryId) private var currentLibraryId
    @Environment(\.continueRow) private var continueRow

    @ViewBuilder var body: some View {
        if work.playable {
            Button {
                play(fromStart: false)
            } label: {
                SwiftUI.Label(work.progress == nil ? String(localized: "Play") : String(localized: "Resume"), systemImage: "play.fill")
            }
            if work.progress != nil {
                Button {
                    play(fromStart: true)
                } label: {
                    SwiftUI.Label(String(localized: "Play from beginning"), systemImage: "arrow.counterclockwise")
                }
            }
        }
        // The card shows the state the server's events send back, so a write
        // that fails leaves it as it was.
        let played = work.userState?.played == true
        Button {
            Haptics.success()
            Task { _ = try? await server.session.library.setPlayed(workId: work.id, played: !played) }
        } label: {
            SwiftUI.Label(Words.markPlayed(!played, kind: work.kind), systemImage: played ? "eye.slash" : "checkmark.circle")
        }
        let favorite = work.userState?.favorite == true
        Button {
            Haptics.success()
            Task { _ = try? await server.session.library.setFavorite(workId: work.id, favorite: !favorite) }
        } label: {
            SwiftUI.Label(
                favorite ? String(localized: "Remove from favorites") : String(localized: "Favorite"),
                systemImage: favorite ? "heart.slash" : "heart"
            )
        }
        if let continueRow {
            Button {
                Haptics.success()
                Task { try? await server.session.library.removeFromContinue(workId: work.id) }
            } label: {
                SwiftUI.Label(
                    continueRow == "continueListening"
                        ? String(localized: "Remove from Continue Listening") : String(localized: "Remove from Continue Watching"),
                    systemImage: "xmark.circle"
                )
            }
        }
        if showsSeries, let episode = work.episode {
            Button {
                openWork(WorkRoute(seriesOf: episode))
            } label: {
                SwiftUI.Label(String(localized: "Go to series"), systemImage: "tv")
            }
        }
        if currentLibraryId == nil, server.library(of: work.libraryId) != nil {
            Button {
                server.showLibrary(of: work.libraryId)
            } label: {
                SwiftUI.Label(String(localized: "Go to library"), systemImage: "square.grid.2x2")
            }
        }
        if ["movie", "episode", "video"].contains(work.kind), work.available, server.session.downloads != nil {
            Section {
                if server.download(of: work.id) != nil {
                    Button(role: .destructive) {
                        server.removeDownload(of: work.id)
                    } label: {
                        SwiftUI.Label(String(localized: "Remove Download"), systemImage: "trash")
                    }
                } else {
                    Button {
                        server.startDownload(work.id)
                    } label: {
                        SwiftUI.Label(String(localized: "Download"), systemImage: "arrow.down.circle")
                    }
                }
            }
        }
    }

    /// Plays its download when one is ready, else from the server; a book in the book player, which chooses for itself.
    private func play(fromStart: Bool) {
        if work.kind == "book" {
            server.listen(work.id, at: fromStart ? 0 : nil)
            return
        }
        server.play(PlayTarget(workId: work.id, fromStart: fromStart, downloadId: server.readyDownload(of: work.id)?.id))
    }
}

extension WorkSummary {
    /// Whether it plays straight from a card: a movie, an episode, a video or a book.
    var playable: Bool {
        ["movie", "episode", "video", "extra", "book"].contains(kind) && available
    }
}
