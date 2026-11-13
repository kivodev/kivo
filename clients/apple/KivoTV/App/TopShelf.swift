import Foundation
import KivoCore
import KivoUI
import TVServices

/// Top Shelf's Continue Watching, as the app keeps it for its extension:
/// read from the server as the app signs in or comes to the front, and soon
/// after a video closes, once its position has reached the server; emptied
/// as the app signs out. Each item's image is kept beside it, since the
/// system can't fetch the server's images itself.
@MainActor
enum TopShelf {
    /// How many items Top Shelf shows at most.
    static let limit = 10

    /// Continue Watching's works, once each, in its order.
    static func shelved(_ home: Home) -> [WorkSummary] {
        var seen = Set<String>()
        let works = home.rows.filter { $0.kind == .continueWatching }.flatMap(\.items)
        return Array(works.filter { seen.insert($0.id).inserted }.prefix(limit))
    }

    /// A work as Top Shelf names it: the movie, or "Show · S1 · E2" for an episode.
    static func title(_ work: WorkSummary) -> String {
        guard let episode = work.episode else { return work.labelledTitle }
        return "\(episode.seriesTitle) · \(Display.app.episodeCode(episode: episode))"
    }

    /// The image Top Shelf shows of a work: the frame where the user stopped, else its backdrop, else its poster.
    static func image(_ work: WorkSummary) -> ImageRef? {
        work.images.still ?? work.images.backdrop ?? work.images.poster
    }

    /// Reads Continue Watching from [session] and keeps it for Top Shelf; a
    /// server that doesn't answer leaves it as it was.
    static func publish(_ session: KivoSession, artwork: ArtworkSource) async {
        guard let home = await session.library.home(kind: .continueWatching).answer(), let folder = TopShelfItems.folder else { return }
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        var items: [TopShelfItem] = []
        for work in shelved(home) {
            var file: String?
            if let image = image(work) {
                let name = "\(image.id).jpg"
                let url = folder.appending(path: name)
                if FileManager.default.fileExists(atPath: url.path(percentEncoded: false)) {
                    file = name
                } else if let data = await artwork.image(id: image.id, pixelWidth: 800)?.jpegData(compressionQuality: 0.85),
                          (try? data.write(to: url, options: .atomic)) != nil {
                    file = name
                }
            }
            items.append(TopShelfItem(id: work.id, title: title(work), image: file, progress: work.progress))
        }
        guard items != TopShelfItems.read() else { return }
        try? TopShelfItems.write(items)
        TVTopShelfContentProvider.topShelfContentDidChange()
    }

    /// Empties Top Shelf, as the app leaves its server.
    static func clear() {
        guard !TopShelfItems.read().isEmpty else { return }
        try? TopShelfItems.write([])
        TVTopShelfContentProvider.topShelfContentDidChange()
    }
}
