import KivoCore
import KivoUI
import SwiftUI

/// A person's page: their photo or initials, their name and
/// biography, then their movies, shows and other videos, newest first,
/// their series, each a row of its books in order, their other books, and
/// the books they narrated. Their photo and name show at once where a
/// list read lately showed them.
struct PersonDetail: View {
    let id: String
    /// Opened from an author's card, so the page's rows are of books.
    var author = false
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @State private var person: Person?
    @State private var failure: RequestProblem?
    @State private var attempt = 0

    var body: some View {
        Group {
            if let heading = person.map({ Heading(id: $0.id, name: $0.name, image: $0.photo) }) ?? server.session.library.knownPerson(id: id) {
                List {
                    header(heading)
                        .detailRow(Self.headerInsets)
                    if let failure {
                        InlineFailure(title: String(localized: "Couldn't load this page."), problem: failure) { attempt += 1 }
                    }
                    if let person {
                        Overview(text: person.overview)
                        CardRow(title: String(localized: "Movies"), works: person.works.filter { $0.kind == .movie })
                        CardRow(title: String(localized: "Shows"), works: person.works.filter { $0.kind == .series })
                        CardRow(title: String(localized: "Other videos"), works: person.works.filter { $0.kind == .video })
                        ForEach(person.bookSeries, id: \.id) { series in
                            CardRow(title: series.name, works: series.items)
                        }
                        CardRow(title: person.bookSeries.isEmpty ? String(localized: "Books") : String(localized: "Other books"), works: person.standalone)
                        CardRow(title: String(localized: "Narrated"), works: person.narrated)
                    } else if failure == nil {
                        CardRowSkeleton(style: author ? .square : .poster)
                    }
                    Color.clear.frame(height: KivoTokens.spacing * 6).detailRow()
                }
                .listStyle(.plain)
                .scrollContentBackground(.hidden)
                .environment(\.defaultMinListRowHeight, 0)
            } else if let failure {
                DetailFailed(failure: failure)
            } else if server.unreachable {
                UnreachableView()
            } else {
                skeleton
            }
        }
        .navigationTitle(person?.name ?? server.session.library.knownPerson(id: id)?.name ?? "")
        .toolbarTitleDisplayMode(.inline)
        .environment(\.currentLibraryId, nil)
        .task(id: "\(id)/\(attempt)") {
            failure = nil
            for await load in server.session.library.person(id: id) {
                if let person = load.getOrNull() {
                    failure = nil
                    self.person = person
                } else if let problem = load.problemOrNull(), !problem.isUnreachable {
                    failure = problem
                }
            }
        }
    }

    private static let headerInsets = EdgeInsets(top: 12, leading: 20, bottom: 8, trailing: 20)
    private static let headerSpacing = KivoTokens.spacing * 3
    private static let photoSize: CGFloat = 120
    private static let nameFont = Font.title2.weight(.bold)

    /// The page's skeleton: the photo, name and count, and a row of posters
    /// or covers. Few people have a biography, so it has none.
    private var skeleton: some View {
        List {
            VStack(spacing: Self.headerSpacing) {
                SkeletonBlock(width: Self.photoSize, height: Self.photoSize, radius: Self.photoSize / 2)
                Text(verbatim: " ").font(Self.nameFont).skeletonLine(width: 180, alignment: .center)
                Text(verbatim: " ").font(.footnote).skeletonLine(width: 90, alignment: .center)
            }
            .frame(maxWidth: .infinity)
            .detailRow(Self.headerInsets)
            CardRowSkeleton(style: author ? .square : .poster)
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .scrollDisabled(true)
        .environment(\.defaultMinListRowHeight, 0)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text("Loading…"))
    }

    /// The photo and name, and how many root works and books, its line held until the page is read.
    private func header(_ heading: Heading) -> some View {
        VStack(spacing: Self.headerSpacing) {
            PersonPhoto(heading.image, name: heading.name, size: Self.photoSize)
            Text(heading.name)
                .font(Self.nameFont)
                .foregroundStyle(palette.text)
                .multilineTextAlignment(.center)
                .accessibilityAddTraits(.isHeader)
            if let person {
                let parts = counts(person)
                if !parts.isEmpty {
                    Text(parts.joined(separator: " · "))
                        .font(.footnote)
                        .foregroundStyle(palette.textMuted)
                }
            } else if failure == nil {
                Text(verbatim: " ").font(.footnote).skeletonLine(width: 90, alignment: .center)
            }
        }
        .frame(maxWidth: .infinity)
    }

    /// "12 titles · 3 books · 2 narrated".
    private func counts(_ person: Person) -> [String] {
        let books = Set(person.bookSeries.flatMap { $0.items.map(\.id) } + person.standalone.map(\.id)).count
        let titles = person.works.count
        return [
            titles > 0 ? (titles == 1 ? String(localized: "1 title") : String(localized: "\(titles) titles")) : nil,
            books > 0 ? (books == 1 ? String(localized: "1 book") : String(localized: "\(books) books")) : nil,
            person.narrated.isEmpty ? nil : String(localized: "\(person.narrated.count) narrated"),
        ].compactMap { $0 }
    }
}
