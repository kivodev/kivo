import KivoCore
import KivoUI
import SwiftUI

/// Home: its rows in order, each shown or not, and Reset Home. Each change
/// is sent at once; a refused one says why, with Retry, as does a refused
/// read of the rows.
struct HomeSettingsView: View {
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    /// Every row of Home, in order.
    @State private var rows: [HomeRowInfo] = []
    @State private var failure: String?
    @State private var retrying: (() -> Void)?
    /// Why the rows couldn't be read, when the server refused it.
    @State private var rowsFailure: RequestProblem?
    @State private var rowsAttempt = 0

    private static let symbols: [HomeRowKind: String] = [.continueWatching: "play", .continueListening: "headphones", .recentlyAdded: "clock"]

    var body: some View {
        List {
            Section {
                ForEach(rows.filter { $0.kind != .unknown }, id: \.id) { row in
                    Toggle(isOn: Binding(
                        get: { row.shown },
                        set: { shown in
                            rows = rows.map { $0.id == row.id ? HomeRowInfo(id: $0.id, kind: $0.kind, shown: shown, library: $0.library, list: $0.list, broken: $0.broken) : $0 }
                            save()
                        }
                    )) {
                        Label {
                            VStack(alignment: .leading) {
                                Text(homeRowTitle(kind: row.kind, libraryName: row.library?.name, listName: row.list?.name))
                                if row.broken?.boolValue == true {
                                    Text("These rules no longer work.")
                                        .font(.caption)
                                        .foregroundStyle(palette.danger)
                                }
                            }
                        } icon: {
                            icon(of: row)
                        }
                    }
                }
                .onMove { from, to in
                    var visible = rows.filter { $0.kind != .unknown }
                    visible.move(fromOffsets: from, toOffset: to)
                    var next = visible.makeIterator()
                    rows = rows.map { $0.kind != .unknown ? next.next() ?? $0 : $0 }
                    save()
                }
            }
            if let rowsFailure {
                Section {
                    FailureRow(message: String(localized: "Couldn't load Home. \(rowsFailure.sentence)"), retry: { rowsAttempt += 1 })
                }
            }
            Section {
                Button("Reset Home") { reset() }
            }
            if let failure {
                Section {
                    FailureRow(message: String(localized: "Couldn't save. \(failure)"), retry: { retrying?() })
                }
            }
        }
        .environment(\.editMode, .constant(.active))
        .navigationTitle(String(localized: "Home"))
        .toolbarTitleDisplayMode(.inline)
        .task(id: rowsAttempt) {
            for await load in server.session.library.homeRows() {
                if let read = load.getOrNull() as? [HomeRowInfo] {
                    rows = read
                    rowsFailure = nil
                } else if let problem = load.problemOrNull(), !problem.isUnreachable {
                    rowsFailure = problem
                }
            }
        }
    }

    /// A row's icon: what kind of row it is, a smart list's in the accent.
    @ViewBuilder private func icon(of row: HomeRowInfo) -> some View {
        if row.list?.kind == .smart {
            Image(systemName: "sparkles").foregroundStyle(palette.accent)
        } else {
            Image(systemName: Self.symbols[row.kind] ?? "list.bullet").foregroundStyle(palette.textMuted)
        }
    }

    private func save() {
        let changes = rows.map { HomeRowChange(id: $0.id, shown: $0.shown) }
        run(retry: save) { _ = try await server.session.library.setHomeRows(rows: changes) }
    }

    private func reset() {
        run(retry: reset) { rows = try await server.session.library.resetHomeRows() }
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
