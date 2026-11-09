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
                Button(String(localized: "Home")) { server?.choose(.home) }
                    .keyboardShortcut("1")
                Button(String(localized: "Lists")) { server?.choose(.lists) }
                    .keyboardShortcut("2")
                if let server {
                    ForEach(Array(server.pinnedLibraries.prefix(6).enumerated()), id: \.element.id) { index, library in
                        Button(library.name) { server.show(library) }
                            .keyboardShortcut(KeyEquivalent(Character(String(index + 3))))
                    }
                }
                Divider()
                Button(String(localized: "Downloads")) { server?.choose(.downloads) }
                    .keyboardShortcut("d", modifiers: [.command, .option])
                Button(String(localized: "Search")) { server?.choose(.search) }
                    .keyboardShortcut("f")
            }
            .disabled(server == nil)
        }
    }
}

/// The Playback menu while the video player shows: Play or Pause on Space,
/// Back 10 seconds and Forward 30 seconds on the arrows, and Close on ⌘-period.
struct PlaybackCommands: Commands {
    let app: AppModel

    var body: some Commands {
        CommandMenu(String(localized: "Playback")) {
            let server = app.server.flatMap { $0.playerShown && $0.playback != nil ? $0 : nil }
            Group {
                Button(String(localized: "Play or pause")) { server?.playback?.togglePlay() }
                    .keyboardShortcut(.space, modifiers: [])
                Button(String(localized: "Back 10 seconds")) { server?.playback?.skip(by: -Markers.shared.BACK_MS) }
                    .keyboardShortcut(.leftArrow, modifiers: [])
                Button(String(localized: "Forward 30 seconds")) { server?.playback?.skip(by: Markers.shared.FORWARD_MS) }
                    .keyboardShortcut(.rightArrow, modifiers: [])
                Divider()
                Button(String(localized: "Close player")) { server?.playerShown = false }
                    .keyboardShortcut(".", modifiers: .command)
            }
            .disabled(server == nil)
        }
    }
}
