import KivoCore
import SwiftUI

/// Favorite and Mark as Watched again, for the ⋯ menu.
struct UserStateMenuItems: View {
    let work: WorkDetail
    @Environment(ServerModel.self) private var server

    var body: some View {
        let favorite = work.userState?.favorite == true
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
            Haptics.success()
            Task { _ = try? await server.session.library.setFavorite(workId: work.id, favorite: !favorite) }
        } label: {
            Label(favorite ? String(localized: "Remove from favorites") : String(localized: "Favorite"), systemImage: favorite ? "heart.slash" : "heart")
        }
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
