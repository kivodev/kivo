import KivoCore
import KivoUI
import SwiftUI

/// Settings: who's signed in, opening Account; a page per topic, each
/// holding the account's settings, kept on the server for every app the
/// user signs in on, and this Apple TV's own; Home and Libraries; and
/// About. Each change applies at once and says nothing unless it fails.
struct SettingsView: View {
    @Environment(ServerModel.self) private var server

    var body: some View {
        List {
            Section {
                NavigationLink { AccountSettingsView() } label: {
                    VStack(alignment: .leading, spacing: KivoTokens.spacing) {
                        Text(server.user?.username ?? "")
                        Text(String(localized: "Signed in to \(server.serverName)"))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
            Section {
                NavigationLink(String(localized: "Playback")) { PlaybackSettingsView() }
                NavigationLink(String(localized: "Audio")) { AudioSettingsView() }
                NavigationLink(String(localized: "Subtitles")) { SubtitleSettingsView() }
                NavigationLink(String(localized: "Quality")) { QualitySettingsView() }
                NavigationLink(String(localized: "Home")) { HomeRowsSettingsView() }
                NavigationLink(String(localized: "Libraries")) { LibrariesSettingsView() }
            }
            Section {
                NavigationLink(String(localized: "About")) { AboutView() }
            }
        }
        .frame(maxWidth: 1100)
    }
}

/// Account: who's signed in, to which server, then Switch account and
/// Change server, which sign this Apple TV out for a sign-in code or for
/// choosing a server, Link a device, and Sign out. Each sign-out asks first.
private struct AccountSettingsView: View {
    private enum Action: Identifiable {
        case switchAccount, changeServer, signOut
        var id: Self { self }
    }

    @Environment(AppModel.self) private var app
    @Environment(ServerModel.self) private var server
    @State private var confirming: Action?

    var body: some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing * 10) {
            Text("\(server.user?.username ?? "") · \(server.serverName)")
                .font(.headline)
                .foregroundStyle(.secondary)
                .padding(.horizontal, KivoTokens.spacing * 4)
            List {
                Button(String(localized: "Switch account")) { confirming = .switchAccount }
                Button(String(localized: "Change server")) { confirming = .changeServer }
                NavigationLink(String(localized: "Link a device")) { LinkDeviceView() }
                Button(String(localized: "Sign out")) { confirming = .signOut }
            }
        }
        .frame(maxWidth: 1100)
        .navigationTitle(String(localized: "Account"))
        .confirmationDialog(title, isPresented: Binding(get: { confirming != nil }, set: { if !$0 { confirming = nil } }), titleVisibility: .visible, presenting: confirming) { action in
            Button(label(action), role: .destructive) {
                if action == .changeServer { app.core.changeServer() }
                Task { await server.signOut() }
            }
            Button(String(localized: "Cancel"), role: .cancel) {}
        } message: { action in
            switch action {
            case .switchAccount: Text("Signs out this Apple TV and shows a sign-in code.")
            case .changeServer: Text("Signs out this Apple TV and looks for servers.")
            case .signOut: EmptyView()
            }
        }
    }

    private var title: String {
        switch confirming {
        case .switchAccount: String(localized: "Switch account?")
        case .changeServer: String(localized: "Change server?")
        case .signOut, nil: ThisDevice.signOutQuestion
        }
    }

    private func label(_ action: Action) -> String {
        switch action {
        case .switchAccount: String(localized: "Switch account")
        case .changeServer: String(localized: "Change server")
        case .signOut: String(localized: "Sign out")
        }
    }
}

/// [all] with the item [id] swapped with the next one up ([by] -1) or down
/// (1) among those [listed], the others staying where they are; [all]
/// itself when there's none.
func moved<Item>(_ all: [Item], id: String, by: Int, of key: (Item) -> String, listed: (Item) -> Bool) -> [Item] {
    let shown = all.filter(listed)
    guard let from = shown.firstIndex(where: { key($0) == id }), shown.indices.contains(from + by),
          let a = all.firstIndex(where: { key($0) == id }),
          let b = all.firstIndex(where: { key($0) == key(shown[from + by]) })
    else { return all }
    var result = all
    result.swapAt(a, b)
    return result
}

/// A row of the Home or Libraries page: its switch, with Move up and Move down beside it.
private struct OrderedRow<Title: View>: View {
    let on: Binding<Bool>
    let canMoveUp: Bool
    let canMoveDown: Bool
    let move: (Int) -> Void
    @ViewBuilder let title: () -> Title

    var body: some View {
        HStack(spacing: KivoTokens.spacing * 6) {
            Toggle(isOn: on, label: title)
            Button { move(-1) } label: {
                Image(systemName: "chevron.up").accessibilityLabel(Text("Move up"))
            }
            .disabled(!canMoveUp)
            Button { move(1) } label: {
                Image(systemName: "chevron.down").accessibilityLabel(Text("Move down"))
            }
            .disabled(!canMoveDown)
        }
    }
}

/// Home: the rows the TV shows in order, each shown or not and moved up or
/// down, and Reset Home, kept on the server. Each change shows at once and
/// goes back with why if the server refuses it.
private struct HomeRowsSettingsView: View {
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    /// Every row of Home, in order, though the TV lists only the rows it shows.
    @State private var rows: [HomeRowInfo] = []
    @State private var failure: SaveFailure?

    /// The rows the TV lists: the kinds Home shows here, and no audiobooks library's.
    private static func listed(_ row: HomeRowInfo) -> Bool {
        ShownRow.kinds.contains(row.kind) && row.library?.kind != .audiobooks
    }

    var body: some View {
        let listed = rows.filter(Self.listed)
        List {
            Section {
                ForEach(listed, id: \.id) { row in
                    OrderedRow(
                        on: Binding(get: { row.shown }, set: { shown in
                            change { $0.map { $0.id == row.id ? HomeRowInfo(id: $0.id, kind: $0.kind, shown: shown, library: $0.library, list: $0.list, broken: $0.broken) : $0 } }
                        }),
                        canMoveUp: row.id != listed.first?.id,
                        canMoveDown: row.id != listed.last?.id,
                        move: { by in change { moved($0, id: row.id, by: by, of: \.id, listed: Self.listed) } }
                    ) {
                        Text(homeRowTitle(kind: row.kind, libraryName: row.library?.name, listName: row.list?.name))
                    }
                }
            }
            Section {
                Button(String(localized: "Reset Home")) { reset() }
            }
            if let failure {
                Section { FailureRow(message: String(localized: "Couldn't save. \(failure.message)"), retry: failure.retry) }
            }
        }
        .frame(maxWidth: 1300)
        .navigationTitle(String(localized: "Home"))
        .task {
            for await load in server.session.library.homeRows() {
                if let read = load.getOrNull() as? [HomeRowInfo] { rows = read }
            }
        }
    }

    private func change(_ transform: ([HomeRowInfo]) -> [HomeRowInfo]) {
        let before = rows
        rows = transform(rows)
        let after = rows
        save { try await server.session.library.setHomeRows(rows: after.map { HomeRowChange(id: $0.id, shown: $0.shown) }) } undo: { rows = before }
    }

    private func reset() {
        save {
            let done = try await server.session.library.resetHomeRows()
            if let reset = done.getOrNull() as? [HomeRowInfo] { rows = reset }
            return done
        } undo: {}
    }

    private func save<T: AnyObject>(_ send: @escaping () async throws -> Done<T>, undo: @escaping () -> Void) {
        Task {
            guard let done = try? await send() else { return }
            if let problem = done.problemOrNull() {
                undo()
                failure = SaveFailure(message: problem.sentence) { save(send, undo: undo) }
            } else {
                failure = nil
            }
        }
    }
}

/// Libraries: the libraries the TV shows in the user's order, each pinned
/// to the tab bar or under More libraries, and moved up or down, kept on
/// the server with the rest, audiobooks too, where they were.
private struct LibrariesSettingsView: View {
    @Environment(ServerModel.self) private var server
    @State private var order: [Library] = []
    @State private var pinned: Set<String> = []
    @State private var failure: SaveFailure?

    private static func listed(_ library: Library) -> Bool {
        library.kind != .audiobooks
    }

    var body: some View {
        let listed = order.filter(Self.listed)
        List {
            Section {
                ForEach(listed, id: \.id) { library in
                    OrderedRow(
                        on: Binding(get: { pinned.contains(library.id) }, set: { pin in
                            if pin { pinned.insert(library.id) } else { pinned.remove(library.id) }
                            save()
                        }),
                        canMoveUp: library.id != listed.first?.id,
                        canMoveDown: library.id != listed.last?.id,
                        move: { by in
                            order = moved(order, id: library.id, by: by, of: \.id, listed: Self.listed)
                            save()
                        }
                    ) {
                        Label(library.name, systemImage: library.symbol)
                    }
                }
            } footer: {
                Text("Libraries turned off are listed under More libraries.")
            }
            if let failure {
                Section { FailureRow(message: String(localized: "Couldn't save. \(failure.message)"), retry: failure.retry) }
            }
        }
        .frame(maxWidth: 1300)
        .navigationTitle(String(localized: "Libraries"))
        .onAppear {
            order = server.libraries
            pinned = Set(server.libraries.filter(\.pinned).map(\.id))
        }
    }

    private func save() {
        let choices = order.map { LibraryChoice(id: $0.id, pinned: pinned.contains($0.id)) }
        Task {
            guard let done = try? await server.session.account.setLibraries(choices: choices) else { return }
            if let problem = done.problemOrNull() {
                failure = SaveFailure(message: problem.sentence, retry: save)
            } else {
                failure = nil
                server.reloadLibraries()
            }
        }
    }
}
