import KivoCore
import KivoUI
import SwiftUI

/// Home & Libraries: Home's rows in order, each shown or not, Reset Home,
/// then every library the user sees, in their order, each pinned or not.
/// Pinned libraries are first in the Libraries menu, in this order; the
/// rest are under More libraries. Each change is sent at once; a refused
/// one says why, with Retry.
struct LibrariesSettingsView: View {
    @Environment(ServerModel.self) private var server
    /// Every row of Home, in order.
    @State private var rows: [HomeRowInfo] = []
    @State private var order: [Library] = []
    @State private var pinned: Set<String> = []
    @State private var failure: String?
    @State private var retrying: (() -> Void)?

    /// The row kinds this app shows; a kind it doesn't know is left out of the editor.
    private static let knownRows: Set<String> = ["continueWatching", "continueListening", "recentlyAdded", "favorites"]

    var body: some View {
        List {
            Section {
                ForEach(rows.filter { Self.knownRows.contains($0.kind) }, id: \.id) { row in
                    Toggle(isOn: Binding(
                        get: { row.shown },
                        set: { shown in
                            rows = rows.map { $0.id == row.id ? HomeRowInfo(id: $0.id, kind: $0.kind, shown: shown, library: $0.library) : $0 }
                            saveRows()
                        }
                    )) {
                        Text(homeRowTitle(kind: row.kind, libraryName: row.library?.name))
                    }
                }
                .onMove { from, to in
                    var visible = rows.filter { Self.knownRows.contains($0.kind) }
                    visible.move(fromOffsets: from, toOffset: to)
                    var next = visible.makeIterator()
                    rows = rows.map { Self.knownRows.contains($0.kind) ? next.next() ?? $0 : $0 }
                    saveRows()
                }
            } header: {
                Text("Home")
            }
            Section {
                Button("Reset Home") { reset() }
            }
            Section {
                ForEach(order, id: \.id) { library in
                    Toggle(isOn: Binding(
                        get: { pinned.contains(library.id) },
                        set: { pin in
                            if pin { pinned.insert(library.id) } else { pinned.remove(library.id) }
                            saveLibraries()
                        }
                    )) {
                        Label(library.name, systemImage: library.symbol)
                    }
                }
                .onMove { from, to in
                    order.move(fromOffsets: from, toOffset: to)
                    saveLibraries()
                }
            } header: {
                Text("Libraries")
            } footer: {
                Text("Libraries turned off are listed under More libraries.")
            }
            if let failure {
                Section {
                    FailureRow(message: String(localized: "Couldn't save. \(failure)"), retry: { retrying?() })
                }
            }
        }
        .environment(\.editMode, .constant(.active))
        .navigationTitle(String(localized: "Home & Libraries"))
        .toolbarTitleDisplayMode(.inline)
        .onAppear {
            order = server.libraries
            pinned = Set(server.libraries.filter(\.pinned).map(\.id))
        }
        .task {
            for await load in server.session.library.homeRows() {
                if let read = load.getOrNull() as? [HomeRowInfo] { rows = read }
            }
        }
    }

    private func saveRows() {
        let changes = rows.map { HomeRowChange(id: $0.id, shown: $0.shown) }
        run(retry: saveRows) { _ = try await server.session.library.setHomeRows(rows: changes) }
    }

    private func reset() {
        run(retry: reset) { rows = try await server.session.library.resetHomeRows() }
    }

    private func saveLibraries() {
        let choices = order.map { LibraryChoice(id: $0.id, pinned: pinned.contains($0.id)) }
        run(retry: saveLibraries) {
            _ = try await server.session.account.setLibraries(choices: choices)
            server.reloadLibraries()
        }
    }

    private func run(retry: @escaping () -> Void, _ call: @escaping () async throws -> Void) {
        retrying = retry
        Task {
            do {
                try await call()
                failure = nil
            } catch {
                failure = error.problemSentence
            }
        }
    }
}
