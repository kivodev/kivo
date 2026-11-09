import KivoCore
import SwiftUI

/// Mark as Watched again and Add to List, for the ⋯ menu.
struct UserStateMenuItems: View {
    let work: WorkDetail
    @Environment(ServerModel.self) private var server

    var body: some View {
        let played = work.userState?.played == true
        // The menu shows the state the server's events send back, so a write
        // that fails leaves it as it was.
        Button {
            Haptics.success()
            Task { _ = try? await server.session.library.setPlayed(workId: work.id, played: !played) }
        } label: {
            Label(Words.markPlayed(!played, kind: work.kind), systemImage: played ? "eye.slash" : "checkmark.circle")
        }
        Button {
            server.addingToList = work.id
        } label: {
            Label(String(localized: "Add to List"), systemImage: "text.badge.plus")
        }
        .accessibilityIdentifier("add-to-list")
    }
}

/// Go to Library, for the ⋯ menu, while the work is in one the user sees.
struct GoToLibraryItem: View {
    let libraryId: String?
    @Environment(ServerModel.self) private var server

    var body: some View {
        if server.library(of: libraryId) != nil {
            Button {
                server.showLibrary(of: libraryId)
            } label: {
                Label(String(localized: "Go to library"), systemImage: "square.grid.2x2")
            }
        }
    }
}
