import KivoCore
import KivoUI
import SwiftUI

/// The rows under a work's hero: more like this, trailers, cast, extras
/// and, with [collections], the collections it's in; a row with nothing in
/// it is left out.
struct DetailRows: View {
    let work: WorkDetail
    var collections = true
    @Environment(\.openWork) private var openWork
    @Environment(\.openURL) private var openURL

    var body: some View {
        WorkRow(title: String(localized: "More like this"), works: work.similar ?? [])
        trailers
        cast
        WorkRow(title: String(localized: "Extras"), works: work.otherExtras())
        if collections {
            ForEach(work.collections, id: \.id) { collection in
                CardRow(title: collection.name) {
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

    /// The trailer files in the work's folder, then its trailers on YouTube, which open in the YouTube app.
    @ViewBuilder private var trailers: some View {
        let local = work.localTrailers()
        let online = work.youTubeTrailers()
        if !local.isEmpty || !online.isEmpty {
            let width = TVLayout.rowWidth(.landscape)
            CardRow(title: String(localized: "Trailers")) {
                ForEach(local, id: \.id) { trailer in
                    WorkLockup(work: trailer, style: .landscape, width: width)
                }
                ForEach(online, id: \.id) { trailer in
                    if let url = URL(string: trailer.url) {
                        Button {
                            openURL(url)
                        } label: {
                            TrailerCard(trailer, width: width)
                        }
                        .buttonStyle(.borderless)
                    }
                }
            }
        }
    }

    /// Each person with their character or role, opening their page.
    @ViewBuilder private var cast: some View {
        if !work.credits.isEmpty {
            CardRow(title: String(localized: "Cast & Crew")) {
                ForEach(Array(work.credits.enumerated()), id: \.offset) { _, credit in
                    Button {
                        openWork(WorkRoute(id: credit.personId, page: .person))
                    } label: {
                        PersonLockup(name: credit.name, photo: credit.photo, detail: credit.character ?? Words.role(credit.knownRole, credit.role))
                    }
                    .buttonStyle(PersonLockup.Style())
                }
            }
        }
    }
}

/// A person's page: their photo, name and how many titles, their
/// biography, then their movies, shows and other videos. Their photo and
/// name show at once where a list read lately showed them.
struct PersonView: View {
    let id: String
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @State private var person: Person?
    @State private var failure: RequestProblem?

    var body: some View {
        let heading = person.map { Heading(id: $0.id, name: $0.name, image: $0.photo) } ?? server.session.library.knownPerson(id: id)
        ScrollView {
            VStack(alignment: .leading, spacing: TVLayout.rowGap) {
                if let heading {
                    HStack(alignment: .center, spacing: KivoTokens.spacing * 12) {
                        PersonPhoto(heading.image, name: heading.name, size: 240)
                        VStack(alignment: .leading, spacing: KivoTokens.spacing * 3) {
                            Text(heading.name)
                                .kivoStyle(KivoTokens.TextSize.xl2, weight: KivoTokens.Weight.bold)
                                .foregroundStyle(palette.text)
                            if let person {
                                let titles = person.works.count
                                Text(titles == 1 ? String(localized: "1 title") : String(localized: "\(titles) titles"))
                                    .kivoStyle(KivoTokens.TextSize.sm)
                                    .foregroundStyle(palette.textMuted)
                                if let overview = person.overview {
                                    Text(overview)
                                        .kivoStyle(KivoTokens.TextSize.sm)
                                        .foregroundStyle(palette.text.opacity(0.85))
                                        .lineLimit(4)
                                        .frame(maxWidth: 1100, alignment: .leading)
                                }
                            }
                        }
                    }
                }
                if let failure {
                    MessageView(String(localized: "Couldn't load this page."), sentence: failure.sentence)
                        .frame(maxWidth: .infinity)
                } else if let person {
                    WorkRow(title: String(localized: "Movies"), works: person.works.filter { $0.kind == .movie })
                    WorkRow(title: String(localized: "Shows"), works: person.works.filter { $0.kind == .series })
                    WorkRow(title: String(localized: "Other videos"), works: person.works.filter { $0.kind == .video })
                } else {
                    SkeletonCardRow(style: .poster)
                }
            }
        }
        .scrollClipDisabled()
        .task {
            for await load in server.session.library.person(id: id) {
                if let read = load.getOrNull() {
                    failure = nil
                    person = read
                } else if let problem = load.problemOrNull(), !problem.isUnreachable {
                    failure = problem
                }
            }
        }
    }
}

/// A collection: its name and how many it holds, at once when a card
/// showed them, over its backdrop once read, then its works as posters.
struct CollectionView: View {
    let id: String
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @State private var collection: KivoCore.Collection?
    @State private var failure: RequestProblem?

    var body: some View {
        let known = server.session.library.knownCollection(id: id)
        DetailPage(art: collection?.images.backdrop ?? collection?.images.poster) {
            VStack(alignment: .leading, spacing: KivoTokens.spacing * 3) {
                Text(collection?.name ?? known?.name ?? " ")
                    .kivoStyle(KivoTokens.TextSize.xl3, weight: KivoTokens.Weight.bold)
                    .foregroundStyle(palette.text)
                let count = collection.map { $0.items.count } ?? known.map { Int($0.workCount) }
                Text(count.map { $0 == 1 ? String(localized: "1 title") : String(localized: "\($0) titles") } ?? " ")
                    .kivoStyle(KivoTokens.TextSize.sm)
                    .foregroundStyle(palette.textMuted)
            }
        } rows: {
            if let failure {
                MessageView(String(localized: "Couldn't load this page."), sentence: failure.sentence)
                    .frame(maxWidth: .infinity)
            } else if let collection {
                LazyVGrid(columns: Array(repeating: GridItem(.fixed(TVLayout.rowWidth(.poster)), spacing: TVLayout.cardGap, alignment: .top), count: 6), alignment: .leading, spacing: TVLayout.lineGap) {
                    ForEach(collection.items, id: \.id) { work in
                        WorkLockup(work: work, style: .poster, width: TVLayout.rowWidth(.poster))
                    }
                }
                .focusSection()
            } else {
                SkeletonCardRow(style: .poster)
            }
        }
        .task {
            for await load in server.session.library.collection(id: id) {
                if let read = load.getOrNull() {
                    failure = nil
                    collection = read
                } else if let problem = load.problemOrNull(), !problem.isUnreachable {
                    failure = problem
                }
            }
        }
    }
}
