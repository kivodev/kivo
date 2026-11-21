import KivoCore
import SwiftUI

/// The tabs in the View menu of the iPad's menu bar, each with its shortcut
/// on a hardware keyboard: Home, Lists, the pinned libraries, Downloads
/// and Search, while signed in.
struct TabCommands: Commands {
    let app: AppModel

    var body: some Commands {
        CommandGroup(after: .sidebar) {
            let server = app.server
            Group {
                Button(String(localized: "Home")) { server?.navigation.choose(.home) }
                    .keyboardShortcut("1")
                Button(String(localized: "Lists")) { server?.navigation.choose(.lists) }
                    .keyboardShortcut("2")
                if let server {
                    ForEach(Array(server.pinnedLibraries.prefix(6).enumerated()), id: \.element.id) { index, library in
                        Button(library.name) { server.navigation.show(library) }
                            .keyboardShortcut(KeyEquivalent(Character(String(index + 3))))
                    }
                }
                Divider()
                Button(String(localized: "Downloads")) { server?.navigation.choose(.downloads) }
                    .keyboardShortcut("d", modifiers: [.command, .option])
                Button(String(localized: "Search")) { server?.navigation.choose(.search) }
                    .keyboardShortcut("f")
            }
            .disabled(server == nil)
        }
    }
}

/// The Playback menu while the video player or the book's Now Playing
/// shows: Play or Pause on Space, Back 10 seconds and Forward 30 seconds,
/// on the arrows for a video (a book's Now Playing has lists they move
/// through), and Close on ⌘-period.
struct PlaybackCommands: Commands {
    let app: AppModel

    var body: some Commands {
        CommandMenu(String(localized: "Playback")) {
            let player = app.server.flatMap { $0.player.shown ? $0.player.model : nil }
            let book = app.server.flatMap { !$0.player.shown && $0.book.loaded && $0.book.expanded ? $0.book : nil }
            Group {
                Button(String(localized: "Play or pause")) {
                    if let player { player.togglePlay() } else { book?.togglePlay() }
                }
                .keyboardShortcut(.space, modifiers: [])
                Button(String(localized: "Back 10 seconds")) {
                    if let player { player.skip(by: -SKIP_BACK_MS) } else { book?.skipBack() }
                }
                .keyboardShortcut(player == nil ? nil : KeyboardShortcut(.leftArrow, modifiers: []))
                Button(String(localized: "Forward 30 seconds")) {
                    if let player { player.skip(by: SKIP_FORWARD_MS) } else { book?.skipForward() }
                }
                .keyboardShortcut(player == nil ? nil : KeyboardShortcut(.rightArrow, modifiers: []))
                Divider()
                Button(String(localized: "Close player")) {
                    if player != nil { app.server?.player.shown = false } else { book?.expanded = false }
                }
                .keyboardShortcut(".", modifiers: .command)
            }
            .disabled(player == nil && book == nil)
        }
    }
}
