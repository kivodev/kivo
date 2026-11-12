import KivoCore
import KivoUI
import Observation
import SwiftUI

/// A manual list's works in its custom order as rows to drag: a small
/// poster, the title and the year, with the system's handle at the end.
struct ListOrder: View {
    let model: ListOrderModel
    let title: String
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Group {
            if let works = model.works {
                List {
                    ForEach(works, id: \.id) { work in
                        row(work)
                    }
                    .onMove { from, to in model.move(from: from, to: to) }
                }
                .listStyle(.plain)
                .readableRows()
                .scrollContentBackground(.hidden)
                .environment(\.editMode, .constant(.active))
                .accessibilityIdentifier("list-order")
            } else if let failure = model.failure {
                MessageView(String(localized: "Couldn't load \(title)."), sentence: failure)
            } else if server.unreachable {
                UnreachableView()
            } else {
                ProgressView()
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .alert(model.problem ?? "", isPresented: Binding(get: { model.problem != nil }, set: { if !$0 { model.problem = nil } })) {
            Button(String(localized: "OK"), role: .cancel) {}
        }
        .task { await model.load() }
    }

    private func row(_ work: WorkSummary) -> some View {
        HStack(spacing: KivoTokens.spacing * 3) {
            ArtworkImage(CardStyle.poster.artwork(of: work), title: work.title, pointWidth: Self.posterWidth)
                .frame(width: Self.posterWidth, height: Self.posterWidth / KivoTokens.Aspect.poster)
                .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.sm))
            VStack(alignment: .leading, spacing: 2) {
                Text(work.title)
                    .font(.body)
                    .foregroundStyle(palette.text)
                    .lineLimit(2)
                if let year = work.gridDetail {
                    Text(year)
                        .font(.subheadline)
                        .monospacedDigit()
                        .foregroundStyle(palette.textMuted)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .accessibilityElement(children: .combine)
        .listRowBackground(Color.clear)
    }

    private static let posterWidth: CGFloat = 40
}

/// A manual list's works in its custom order, and the moves made in it:
/// each shows at once and is sent in turn, and one the server refuses is
/// undone by reading the order again.
@MainActor
@Observable
final class ListOrderModel {
    /// The works in order, nil until they're read.
    private(set) var works: [WorkSummary]?
    /// Why the works couldn't be read.
    private(set) var failure: String?
    /// Why a move wasn't kept.
    var problem: String?
    @ObservationIgnored private let listId: String
    @ObservationIgnored private let session: KivoSession
    /// The last move sent, which the next waits for.
    @ObservationIgnored private var sending: Task<Void, Never>?

    init(listId: String, session: KivoSession) {
        self.listId = listId
        self.session = session
    }

    func load() async {
        do {
            works = try await session.library.listWorksInCustomOrder(listId: listId)
            failure = nil
        } catch {
            guard !Task.isCancelled else { return }
            if works == nil { failure = error.problemSentence }
        }
    }

    /// Moves the rows at [from] to before the row at [to], as a list's `onMove` gives them.
    func move(from: IndexSet, to: Int) {
        guard var order = works else { return }
        let moved = from.map { order[$0].id }
        order.move(fromOffsets: from, toOffset: to)
        guard let first = order.firstIndex(where: { $0.id == moved.first }), order.map(\.id) != works?.map(\.id) else { return }
        works = order
        let after = first > 0 ? order[first - 1].id : nil
        sending = Task { [sending, listId, session] in
            await sending?.value
            guard let failed = try? await session.library.moveInList(listId: listId, workIds: moved, after: after).problemOrNull() else { return }
            problem = failed.sentence
            await load()
        }
    }

    /// Returns once every move made has been sent.
    func settle() async {
        await sending?.value
    }
}
