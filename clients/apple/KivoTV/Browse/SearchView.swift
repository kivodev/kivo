import KivoCore
import KivoUI
import SwiftUI

/// Search: the system's search field and keyboard, then the people who
/// match in a row, a row of results for each kind the TV shows, always in
/// the same order, and the collections that match (SearchModel). While a
/// search runs the last results stay, dimmed.
struct SearchView: View {
    @Environment(ServerModel.self) private var server
    @Environment(\.openWork) private var openWork
    @State private var query = ""
    @State private var model: SearchModel?

    /// The rows of works the TV shows: no books.
    private static let kinds = SearchModel.kinds.filter { !$0.kinds.contains(.book) }

    var body: some View {
        ScrollView {
            let results = model?.results
            if query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                EmptyView()
            } else if let results {
                if results.works.allSatisfy({ $0.kind == .book }) && results.people.isEmpty && results.collections.isEmpty {
                    MessageView(String(localized: "No results for “\(model?.searched ?? "")”."))
                        .frame(maxWidth: .infinity)
                } else {
                    LazyVStack(alignment: .leading, spacing: TVLayout.rowGap) {
                        if !results.people.isEmpty { people(results.people) }
                        ForEach(Self.kinds, id: \.title) { group in
                            WorkRow(title: group.title, works: results.works.filter { group.kinds.contains($0.kind) })
                        }
                        if !results.collections.isEmpty { collections(results.collections) }
                    }
                    .opacity(model?.running == true ? 0.5 : 1)
                }
            } else if model?.failed?.isUnreachable == true {
                UnreachableView()
            } else if let failed = model?.failed {
                MessageView(String(localized: "Search failed."), sentence: failed.sentence)
                    .frame(maxWidth: .infinity)
            }
        }
        .searchable(text: $query, prompt: String(localized: "Movies, shows and people"))
        .task(id: query) {
            if model == nil { model = SearchModel(session: server.session) }
            await model?.search(query)
        }
    }

    /// Each person, round, with the role they have most and how many titles, opening their page.
    private func people(_ people: [PersonSummary]) -> some View {
        CardRow(title: String(localized: "People")) {
            ForEach(people, id: \.id) { person in
                Button {
                    openWork(WorkRoute(id: person.id, page: .person))
                } label: {
                    PersonLockup(name: person.name, photo: person.photo, detail: SearchModel.detail(person))
                }
                .buttonStyle(.borderless)
            }
        }
    }

    /// Each collection's poster, name and how many works, opening its page.
    private func collections(_ collections: [CollectionSummary]) -> some View {
        CardRow(title: String(localized: "Collections")) {
            ForEach(collections, id: \.id) { collection in
                Button {
                    openWork(WorkRoute(id: collection.id, page: .collection))
                } label: {
                    GroupCard(collection, width: TVLayout.rowWidth(.poster))
                }
                .buttonStyle(.borderless)
            }
        }
    }
}

/// A person's round photo, lifting with focus, over their name and a line about them.
struct PersonLockup: View {
    let name: String
    let photo: ImageRef?
    let detail: String?
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        VStack(spacing: KivoTokens.spacing * 3) {
            PersonPhoto(photo, name: name, size: 180)
                .cardLift()
            VStack(spacing: 2) {
                Text(name)
                    .kivoStyle(KivoTokens.TextSize.sm, weight: KivoTokens.Weight.medium)
                    .foregroundStyle(palette.text)
                Text(detail ?? " ")
                    .kivoStyle(KivoTokens.TextSize.xs)
                    .foregroundStyle(palette.textMuted)
            }
            .lineLimit(1)
        }
        .frame(width: 200)
    }
}
