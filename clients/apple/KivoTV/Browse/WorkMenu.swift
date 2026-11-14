import KivoCore
import SwiftUI

/// A work's menu, which holding Select on its card opens: Play or Resume,
/// Play from beginning once there's progress, Mark as watched or
/// unwatched, Remove from the manual list it's shown in, Remove from
/// Continue Watching in that row, Go to series for an episode, and Go to
/// library outside that library's grid.
struct WorkMenu: View {
    let work: WorkSummary
    @Environment(ServerModel.self) private var server
    @Environment(\.openWork) private var openWork
    @Environment(\.currentLibraryId) private var currentLibraryId
    @Environment(\.continueRow) private var continueRow
    @Environment(\.currentList) private var currentList

    var body: some View {
        let marks = server.session.library.marks
        if work.playable {
            Button {
                server.play(work.id)
            } label: {
                Label(work.progress == nil ? String(localized: "Play") : String(localized: "Resume"), systemImage: "play.fill")
            }
            if work.progress != nil {
                Button {
                    server.play(work.id, fromStart: true)
                } label: {
                    Label(String(localized: "Play from beginning"), systemImage: "arrow.counterclockwise")
                }
            }
        }
        // The card shows the state the server's events send back, so a mark
        // that fails leaves it as it was, and the app says why.
        let played = work.userState?.played == true
        Button {
            marks.setPlayed(workId: work.id, played: !played)
        } label: {
            Label(Words.markPlayed(!played, kind: work.kind), systemImage: played ? "eye.slash" : "checkmark.circle")
        }
        if let currentList, currentList.kind == .manual {
            Button {
                marks.setOnList(listId: currentList.id, workId: work.id, on: false)
            } label: {
                Label(String(localized: "Remove from \(currentList.name)"), systemImage: "minus.circle")
            }
        }
        if continueRow == .continueWatching {
            Button {
                marks.removeFromContinue(workId: work.id)
            } label: {
                Label(String(localized: "Remove from Continue Watching"), systemImage: "xmark.circle")
            }
        }
        if let episode = work.episode {
            Button {
                openWork(WorkRoute(seriesOf: episode))
            } label: {
                Label(String(localized: "Go to series"), systemImage: "tv")
            }
        }
        if work.libraryId != currentLibraryId, let library = server.library(of: work.libraryId) {
            Button {
                server.navigation.show(library)
            } label: {
                Label(String(localized: "Go to library"), systemImage: "square.grid.2x2")
            }
        }
    }
}
