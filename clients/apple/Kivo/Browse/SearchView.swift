import KivoCore
import KivoUI
import SwiftUI

/// Search, below the tab's field: the people who match in a
/// row, then a row of results for each kind, always in the same order, a
/// kind with no matches having no row, then the collections that match. It
/// runs once typing pauses for about 350 ms; meanwhile the last results
/// stay, dimmed. Coming back from a result finds them as they were, without
/// searching again.
struct SearchView: View {
    @Binding var query: String
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @Environment(\.openWork) private var openWork
    @Environment(\.wide) private var wide
    @State private var results: SearchResults?
    @State private var searched = ""
    @State private var running = false
    /// Why the search failed, as a problem code.
    @State private var failed: RequestProblem?

    private struct KindRow {
        let kinds: Set<String>
        let title: String
    }

    private static let kinds = [
        KindRow(kinds: ["movie"], title: String(localized: "Movies")),
        KindRow(kinds: ["series"], title: String(localized: "Shows")),
        KindRow(kinds: ["episode"], title: String(localized: "Episodes")),
        KindRow(kinds: ["video", "extra"], title: String(localized: "Other videos")),
        KindRow(kinds: ["book"], title: String(localized: "Books")),
    ]

    var body: some View {
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
                        String(localized: "No results for “\(searched)”."),
                        action: (label: String(localized: "Clear search"), run: { query = "" })
                    )
                } else {
                    LazyVStack(alignment: .leading, spacing: KivoTokens.spacing * 6) {
                        if !results.people.isEmpty { peopleRow(results.people) }
                        ForEach(Self.kinds, id: \.title) { group in
                            let matches = results.works.filter { group.kinds.contains($0.kind) }
                            if !matches.isEmpty { row(group.title, matches) }
                        }
                        if !results.collections.isEmpty { collectionsRow(results.collections) }
                    }
                    .padding(.vertical, KivoTokens.spacing * 4)
                    .opacity(running ? 0.5 : 1)
                }
            } else if failed?.isUnreachable == true {
                UnreachableView { Task { await search(query) } }
                    .frame(maxWidth: .infinity)
                    .padding(.top, KivoTokens.spacing * 10)
            } else if let failed {
                MessageView(String(localized: "Search failed."), sentence: failed.sentence)
            }
        }
        .navigationTitle(String(localized: "Search"))
        .task(id: query) { await search(query) }
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
                                    Text(personDetail(person))
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

    /// "Actor · 23 titles".
    private func personDetail(_ person: PersonSummary) -> String {
        let n = Int(person.workCount)
        let titles = n == 1 ? String(localized: "1 title") : String(localized: "\(n) titles")
        return "\(Words.role(person.role)) · \(titles)"
    }

    private func search(_ text: String) async {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else {
            results = nil
            failed = nil
            return
        }
        // Back from a result, the results are this query's already.
        guard trimmed != searched || results == nil else { return }
        do {
            try await Task.sleep(for: .milliseconds(350))
        } catch {
            return
        }
        running = true
        defer { running = false }
        do {
            let found = try await server.session.library.search(query: trimmed, limit: nil)
            guard !Task.isCancelled else { return }
            results = found
            searched = trimmed
            failed = nil
        } catch {
            guard !Task.isCancelled else { return }
            failed = error.requestProblem
            results = nil
        }
    }
}
