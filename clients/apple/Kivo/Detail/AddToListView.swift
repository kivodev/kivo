import KivoCore
import KivoUI
import SwiftUI

/// Add to List: the user's lists, each checked while it holds the work,
/// a tap adding or removing it at once, with New List to make one holding
/// the work. A write that fails puts the row back and says why.
struct AddToListView: View {
    let workId: String
    @Environment(ServerModel.self) private var server
    @Environment(\.dismiss) private var dismiss
    @Environment(\.kivoPalette) private var palette
    /// The lists that hold the work, as read and as tapped since.
    @State private var held: Set<String> = []
    @State private var read = false
    @State private var naming = false
    @State private var newName = ""
    @State private var problem: String?

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Button {
                        newName = ""
                        naming = true
                    } label: {
                        Label(String(localized: "New List"), systemImage: "plus")
                    }
                    .accessibilityIdentifier("new-list")
                }
                Section {
                    ForEach(server.lists, id: \.id) { list in
                        row(list)
                    }
                }
            }
            .scrollContentBackground(.hidden)
            .navigationTitle(String(localized: "Add to List"))
            .toolbarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button(String(localized: "Done")) { dismiss() }
                }
            }
            .kivoPageBackground()
        }
        .presentationDetents([.medium, .large])
        .listNameAlert(String(localized: "New List"), isPresented: $naming, name: $newName, confirm: String(localized: "Create"), save: create)
        .alert(problem ?? "", isPresented: Binding(get: { problem != nil }, set: { if !$0 { problem = nil } })) {
            Button(String(localized: "OK"), role: .cancel) {}
        }
        .task {
            for await load in server.session.library.work(id: workId) {
                guard let work = load.getOrNull() else { continue }
                held = Set(work.listIds ?? [])
                read = true
            }
        }
    }

    private func row(_ list: WorkList) -> some View {
        Button {
            toggle(list)
        } label: {
            HStack(spacing: KivoTokens.spacing * 3) {
                VStack(alignment: .leading, spacing: KivoTokens.spacing * 0.5) {
                    Text(list.name)
                        .foregroundStyle(palette.text)
                    Text(list.titleCount)
                        .font(.footnote)
                        .foregroundStyle(palette.textMuted)
                }
                Spacer()
                if held.contains(list.id) {
                    Image(systemName: "checkmark")
                        .fontWeight(.semibold)
                        .foregroundStyle(palette.accent)
                }
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .disabled(!read)
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(held.contains(list.id) ? .isSelected : [])
        .accessibilityIdentifier("add-to:\(list.name)")
    }

    private func toggle(_ list: WorkList) {
        Haptics.success()
        let adding = !held.contains(list.id)
        if adding { held.insert(list.id) } else { held.remove(list.id) }
        Task {
            do {
                if adding {
                    try await server.session.library.addToList(listId: list.id, workId: workId)
                } else {
                    try await server.session.library.removeFromList(listId: list.id, workId: workId)
                }
            } catch {
                if adding { held.remove(list.id) } else { held.insert(list.id) }
                problem = error.problemSentence
            }
        }
    }

    private func create(_ name: String) {
        Task {
            do {
                let list = try await server.session.library.createList(name: name)
                try await server.session.library.addToList(listId: list.id, workId: workId)
                held.insert(list.id)
            } catch {
                problem = error.listNameSentence
            }
        }
    }
}
