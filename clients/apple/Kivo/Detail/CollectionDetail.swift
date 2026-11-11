import KivoCore
import KivoUI
import SwiftUI

/// A collection's page: a movie collection or a videos library's folder,
/// its poster, name and how many root works, then the root works in release order.
/// The poster, name and count show at once where a card read lately showed
/// them.
struct CollectionDetail: View {
    let id: String
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @State private var collection: KivoCore.Collection?
    @State private var failure: RequestProblem?
    @State private var attempt = 0

    private static let headerInsets = EdgeInsets(top: 20, leading: 20, bottom: 12, trailing: 20)
    private static let nameFont = Font.title2.weight(.bold)
    private static let posterWidth: CGFloat = 120
    private static let cardWidth: CGFloat = 104
    private static let columns = [GridItem(.adaptive(minimum: cardWidth), spacing: KivoTokens.spacing * 2.5, alignment: .top)]

    var body: some View {
        let card = server.session.library.knownCollection(id: id)
        Group {
            if let name = collection?.name ?? card?.name {
                List {
                    header(name, card: card)
                        .detailRow(Self.headerInsets)
                    if let failure {
                        InlineFailure(title: String(localized: "Couldn't load this collection."), problem: failure) { attempt += 1 }
                    } else {
                        LazyVGrid(columns: Self.columns, spacing: KivoTokens.spacing * 4) {
                            if let collection {
                                ForEach(collection.items, id: \.id) { work in
                                    WorkLink(work: work) { WorkCard(work, width: Self.cardWidth, caption: .oneLine) }
                                }
                            } else {
                                ForEach(0..<min(Int(card?.workCount ?? 6), 12), id: \.self) { _ in
                                    SkeletonCard(style: .poster, width: Self.cardWidth, caption: .oneLine)
                                }
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
                LoadingView(String(localized: "Loading…"))
            }
        }
        .navigationTitle(collection?.name ?? card?.name ?? "")
        .toolbarTitleDisplayMode(.inline)
        .task(id: "\(id)/\(attempt)") {
            failure = nil
            for await load in server.session.library.collection(id: id) {
                if let collection = load.getOrNull() {
                    failure = nil
                    self.collection = collection
                } else if let problem = load.problemOrNull(), !problem.isUnreachable {
                    failure = problem
                }
            }
        }
    }

    /// The poster when there is one, the name and how many root works, the
    /// count held until it's known.
    private func header(_ name: String, card: CollectionSummary?) -> some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing) {
            if let poster = collection?.images.poster ?? card?.images.poster {
                ArtworkImage(poster, title: name, pointWidth: Self.posterWidth)
                    .frame(width: Self.posterWidth, height: Self.posterWidth / CardStyle.poster.aspect)
                    .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
                    .kivoShadow(palette.cardShadow)
                    .padding(.bottom, KivoTokens.spacing * 2)
            }
            Text(name)
                .font(Self.nameFont)
                .foregroundStyle(palette.text)
                .accessibilityAddTraits(.isHeader)
            if let count = collection.map({ $0.items.count }) ?? card.map({ Int($0.workCount) }) {
                let movies = (collection?.source ?? card?.source) == .provider
                Text(movies
                    ? (count == 1 ? String(localized: "1 movie") : String(localized: "\(count) movies"))
                    : (count == 1 ? String(localized: "1 item") : String(localized: "\(count) items")))
                    .font(.footnote)
                    .foregroundStyle(palette.textMuted)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
