import KivoCore
import KivoUI
import SwiftUI

/// A book series' page: its first cover on a stack, its name
/// and how many books, then the books in series order. The cover and name
/// show at once where a list read lately showed them.
struct BookSeriesDetail: View {
    let id: String
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @State private var series: BookSeries?
    @State private var failure: RequestProblem?
    @State private var attempt = 0

    var body: some View {
        Group {
            if let heading = series.map({ Heading(id: $0.id, name: $0.name, image: Self.cover(of: $0)) }) ?? server.session.library.knownBookSeries(id: id) {
                List {
                    header(heading)
                        .detailRow(Self.headerInsets)
                    if let failure {
                        InlineFailure(title: String(localized: "Couldn't load this page."), problem: failure) { attempt += 1 }
                    } else {
                        LazyVGrid(columns: Self.columns, spacing: Self.rowSpacing) {
                            if let series {
                                ForEach(series.items, id: \.id) { book in
                                    WorkLink(work: book) { WorkCard(book, style: .square, width: Self.cardWidth) }
                                }
                            } else {
                                ForEach(0..<3, id: \.self) { _ in SkeletonCard(style: .square, width: Self.cardWidth) }
                            }
                        }
                        .detailRow()
                    }
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
        .navigationTitle(series?.name ?? server.session.library.knownBookSeries(id: id)?.name ?? "")
        .toolbarTitleDisplayMode(.inline)
        .environment(\.currentLibraryId, nil)
        .task(id: "\(id)/\(attempt)") {
            failure = nil
            for await load in server.session.library.bookSeries(id: id) {
                if let series = load.getOrNull() {
                    failure = nil
                    self.series = series
                } else if let problem = load.problemOrNull(), !problem.isUnreachable {
                    failure = problem
                }
            }
        }
    }

    private static let headerInsets = EdgeInsets(top: 20, leading: 20, bottom: 12, trailing: 20)
    private static let headerSpacing = KivoTokens.spacing * 3
    private static let coverSize: CGFloat = 160
    private static let nameFont = Font.title2.weight(.bold)
    private static let cardWidth: CGFloat = 104
    private static let columns = [GridItem(.adaptive(minimum: cardWidth), spacing: KivoTokens.spacing * 2.5, alignment: .top)]
    private static let rowSpacing = KivoTokens.spacing * 4

    /// The page's skeleton: the cover with the room its stack takes above
    /// it, the name and count, and a row of covers.
    private var skeleton: some View {
        List {
            VStack(spacing: Self.headerSpacing) {
                SkeletonBlock(width: Self.coverSize, height: Self.coverSize)
                    .padding(.top, 10)
                Text(verbatim: " ").font(Self.nameFont).skeletonLine(width: 200, alignment: .center)
                Text(verbatim: " ").font(.footnote).skeletonLine(width: 60, alignment: .center)
            }
            .frame(maxWidth: .infinity)
            .detailRow(Self.headerInsets)
            LazyVGrid(columns: Self.columns, spacing: Self.rowSpacing) {
                ForEach(0..<3, id: \.self) { _ in SkeletonCard(style: .square, width: Self.cardWidth) }
            }
            .detailRow()
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .scrollDisabled(true)
        .environment(\.defaultMinListRowHeight, 0)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text("Loading…"))
    }

    /// The first book's cover that there is.
    private static func cover(of series: BookSeries) -> ImageRef? {
        series.items.first { $0.images.poster != nil }?.images.poster
    }

    /// The cover on its stack, the name, and how many books, held until the page is read.
    private func header(_ heading: Heading) -> some View {
        VStack(spacing: Self.headerSpacing) {
            if series?.items.isEmpty != true {
                ArtworkImage(heading.image, title: heading.name, pointWidth: Self.coverSize)
                    .frame(width: Self.coverSize, height: Self.coverSize)
                    .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
                    .background(alignment: .top) {
                        ZStack(alignment: .top) {
                            RoundedRectangle(cornerRadius: KivoTokens.Radius.md).fill(palette.surfaceHover)
                                .frame(width: 136, height: 160).offset(y: -10)
                            RoundedRectangle(cornerRadius: KivoTokens.Radius.md).fill(palette.border)
                                .frame(width: 148, height: 160).offset(y: -5)
                        }
                    }
                    .kivoShadow(palette.cardShadow)
                    .padding(.top, 10)
            }
            Text(heading.name)
                .font(Self.nameFont)
                .foregroundStyle(palette.text)
                .multilineTextAlignment(.center)
                .accessibilityAddTraits(.isHeader)
            if let series {
                Text(series.items.count == 1 ? String(localized: "1 book") : String(localized: "\(series.items.count) books"))
                    .font(.footnote)
                    .foregroundStyle(palette.textMuted)
            } else if failure == nil {
                Text(verbatim: " ").font(.footnote).skeletonLine(width: 60, alignment: .center)
            }
        }
        .frame(maxWidth: .infinity)
    }
}
