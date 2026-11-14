import KivoCore
import KivoUI
import SwiftUI

/// Search, below the tab's field: the people who match in a
/// row, then a row of results for each kind, always in the same order, a
/// kind with no matches having no row, then the collections that match
/// (SearchModel). While a search runs the last results stay, dimmed.
struct SearchView: View {
    @Binding var query: String
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @Environment(\.openWork) private var openWork
    @Environment(\.wide) private var wide
    @State private var model: SearchModel?

    var body: some View {
        let results = model?.results
        let failed = model?.failed
        ScrollView {
            if query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                VStack(spacing: KivoTokens.spacing * 3) {
                    Image(systemName: "magnifyingglass")
                        .font(.system(size: 44))
                        .accessibilityHidden(true)
                    Text(String(localized: "Search movies, shows, books and people"))
                        .kivoStyle(KivoTokens.TextSize.base)
                }
                .foregroundStyle(palette.textMuted)
                .multilineTextAlignment(.center)
                .padding(KivoTokens.spacing * 6)
                .containerRelativeFrame([.horizontal, .vertical])
            } else if let results {
                if results.works.isEmpty && results.people.isEmpty && results.collections.isEmpty {
                    MessageView(
                        String(localized: "No results for “\(model?.searched ?? "")”."),
                        action: (label: String(localized: "Clear search"), run: { query = "" })
                    )
                } else {
                    LazyVStack(alignment: .leading, spacing: KivoTokens.spacing * 6) {
                        if !results.people.isEmpty { peopleRow(results.people) }
                        ForEach(SearchModel.kinds, id: \.title) { group in
                            let matches = results.works.filter { group.kinds.contains($0.kind) }
                            if !matches.isEmpty { row(group.title, matches) }
                        }
                        if !results.collections.isEmpty { collectionsRow(results.collections) }
                    }
                    .padding(.vertical, KivoTokens.spacing * 4)
                    .opacity(model?.running == true ? 0.5 : 1)
                }
            } else if failed?.isUnreachable == true {
                UnreachableView { Task { await model?.search(query) } }
                    .frame(maxWidth: .infinity)
                    .padding(.top, KivoTokens.spacing * 10)
            } else if let failed {
                MessageView(String(localized: "Search failed."), sentence: failed.sentence)
            }
        }
        .navigationTitle(String(localized: "Search"))
        .task(id: query) {
            if model == nil { model = SearchModel(session: server.session) }
            await model?.search(query)
        }
    }

    private func row(_ title: String, _ works: [WorkSummary]) -> some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing * 3) {
            Text(title)
                .kivoStyle(KivoTokens.TextSize.lg, weight: KivoTokens.Weight.semibold)
                .foregroundStyle(palette.text)
                .padding(.horizontal, KivoTokens.spacing * 4)
            ScrollView(.horizontal, showsIndicators: false) {
                LazyHStack(alignment: .top, spacing: KivoTokens.spacing * 3) {
                    ForEach(works, id: \.id) { work in
                        let style = CardStyle.of(work)
                        WorkLink(work: work) {
                            WorkCard(work, style: style, width: wide ? BrowseRow.cardWidth(style, wide: true) : style == .landscape ? 220 : 110)
                        }
                    }
                }
                .padding(.horizontal, KivoTokens.spacing * 4)
            }
        }
    }

    /// Each collection's poster, name and how many root works, opening its page.
    private func collectionsRow(_ collections: [CollectionSummary]) -> some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing * 3) {
            Text(String(localized: "Collections"))
                .kivoStyle(KivoTokens.TextSize.lg, weight: KivoTokens.Weight.semibold)
                .foregroundStyle(palette.text)
                .padding(.horizontal, KivoTokens.spacing * 4)
            ScrollView(.horizontal, showsIndicators: false) {
                LazyHStack(alignment: .top, spacing: KivoTokens.spacing * 3) {
                    ForEach(collections, id: \.id) { collection in
                        Button {
                            openWork(WorkRoute(id: collection.id, page: .collection))
                        } label: {
                            GroupCard(collection, width: wide ? BrowseRow.cardWidth(.poster, wide: true) : 110)
                        }
                        .buttonStyle(.plain)
                    }
                }
                .padding(.horizontal, KivoTokens.spacing * 4)
            }
        }
    }

    /// Each person with the role they have most and how many root works, opening their page.
    private func peopleRow(_ people: [PersonSummary]) -> some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing * 3) {
            Text(String(localized: "People"))
                .kivoStyle(KivoTokens.TextSize.lg, weight: KivoTokens.Weight.semibold)
                .foregroundStyle(palette.text)
                .padding(.horizontal, KivoTokens.spacing * 4)
            ScrollView(.horizontal, showsIndicators: false) {
                LazyHStack(alignment: .top, spacing: KivoTokens.spacing * 3) {
                    ForEach(people, id: \.id) { person in
                        Button {
                            openWork(WorkRoute(id: person.id, page: .person))
                        } label: {
                            VStack(spacing: KivoTokens.spacing * 1.5) {
                                PersonPhoto(person.photo, name: person.name, size: 88)
                                VStack(spacing: 1) {
                                    Text(person.name)
                                        .font(.caption.weight(.semibold))
                                        .foregroundStyle(palette.text)
                                    Text(SearchModel.detail(person))
                                        .font(.caption)
                                        .foregroundStyle(palette.textMuted)
                                }
                                .lineLimit(1)
                                .multilineTextAlignment(.center)
                            }
                            .frame(width: 96)
                            .accessibilityElement(children: .combine)
                        }
                        .buttonStyle(.plain)
                    }
                }
                .padding(.horizontal, KivoTokens.spacing * 4)
            }
        }
    }

}
