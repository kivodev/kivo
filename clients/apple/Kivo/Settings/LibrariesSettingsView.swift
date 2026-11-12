import KivoCore
import KivoUI
import SwiftUI

/// Libraries: every library the user sees, in their order, each pinned or
/// not. Pinned libraries are first in the Libraries menu, in this order; the
/// rest are under More libraries. Each change is sent at once; a refused one
/// says why, with Retry.
struct LibrariesSettingsView: View {
    @Environment(ServerModel.self) private var server
    @State private var order: [Library] = []
    @State private var pinned: Set<String> = []
    @State private var failure: String?

    var body: some View {
        List {
            Section {
                ForEach(order, id: \.id) { library in
                    Toggle(isOn: Binding(
                        get: { pinned.contains(library.id) },
                        set: { pin in
                            if pin { pinned.insert(library.id) } else { pinned.remove(library.id) }
                            save()
                        }
                    )) {
                        Label(library.name, systemImage: library.symbol)
                    }
                }
                .onMove { from, to in
                    order.move(fromOffsets: from, toOffset: to)
                    save()
                }
            } footer: {
                Text("Libraries turned off are listed under More libraries.")
            }
            if let failure {
                Section {
                    FailureRow(message: String(localized: "Couldn't save. \(failure)"), retry: save)
                }
            }
        }
        .environment(\.editMode, .constant(.active))
        .navigationTitle(String(localized: "Libraries"))
        .toolbarTitleDisplayMode(.inline)
        .onAppear {
            order = server.libraries
            pinned = Set(server.libraries.filter(\.pinned).map(\.id))
        }
    }

    private func save() {
        let choices = order.map { LibraryChoice(id: $0.id, pinned: pinned.contains($0.id)) }
        Task {
            guard let done = try? await server.session.account.setLibraries(choices: choices) else { return }
            failure = done.problemOrNull()?.sentence
            if failure == nil { server.reloadLibraries() }
        }
    }
}
