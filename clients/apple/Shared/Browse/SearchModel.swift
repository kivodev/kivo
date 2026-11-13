import Foundation
import KivoCore
import Observation

/// Search's results as they come: the search runs once typing pauses for
/// about 350 ms, and meanwhile the last results stay. Coming back from a
/// result finds them as they were, without searching again.
@MainActor
@Observable
final class SearchModel {
    /// A row of results: the works of [kinds], under [title].
    struct KindRow {
        let kinds: Set<WorkKind>
        let title: String
    }

    /// The rows of works, always in this order, each holding the matches of its kinds.
    static let kinds = [
        KindRow(kinds: [.movie], title: String(localized: "Movies")),
        KindRow(kinds: [.series], title: String(localized: "Shows")),
        KindRow(kinds: [.episode], title: String(localized: "Episodes")),
        KindRow(kinds: [.video, .extra], title: String(localized: "Other videos")),
        KindRow(kinds: [.book], title: String(localized: "Books")),
    ]

    private(set) var results: SearchResults?
    /// What the results are for.
    private(set) var searched = ""
    /// Whether a search is under way, over the last results.
    private(set) var running = false
    /// Why the search failed.
    private(set) var failed: RequestProblem?

    private let session: KivoSession

    init(session: KivoSession) {
        self.session = session
    }

    /// Searches for [text] once typing pauses; an empty query clears the results.
    func search(_ text: String) async {
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
            let found = try await session.library.search(query: trimmed, limit: nil)
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

    /// "Actor · 23 titles".
    static func detail(_ person: PersonSummary) -> String {
        let n = Int(person.workCount)
        let titles = n == 1 ? String(localized: "1 title") : String(localized: "\(n) titles")
        return "\(Words.role(person.knownRole, person.role)) · \(titles)"
    }
}
