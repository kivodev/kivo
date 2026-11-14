import Foundation

/// Continue Watching as Top Shelf shows it: the app writes it into the app
/// group it shares with its Top Shelf extension, which reads it there, as
/// the system's Top Shelf can't send the server's images their token.
struct TopShelfItem: Codable, Equatable {
    let id: String
    let title: String
    /// The image's file in the shelf's folder; nil without artwork.
    let image: String?
    /// How far through it the user is, 0 to 1.
    let progress: Double?
}

/// Where Top Shelf's items and their images are kept: the app group's
/// TopShelf folder, which the Info.plist's `KivoAppGroup` names.
enum TopShelfItems {
    static var folder: URL? {
        guard let group = Bundle.main.object(forInfoDictionaryKey: "KivoAppGroup") as? String,
              let container = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: group)
        else { return nil }
        return container.appending(path: "TopShelf", directoryHint: .isDirectory)
    }

    private static var list: URL? { folder?.appending(path: "items.json") }

    /// The items the app last wrote, none before it has.
    static func read() -> [TopShelfItem] {
        guard let list, let data = try? Data(contentsOf: list) else { return [] }
        return (try? JSONDecoder().decode([TopShelfItem].self, from: data)) ?? []
    }

    /// Keeps [items], their images already in the folder, and deletes the images they don't name.
    static func write(_ items: [TopShelfItem]) throws {
        guard let folder, let list else { return }
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        try JSONEncoder().encode(items).write(to: list, options: .atomic)
        let kept = Set(items.compactMap(\.image) + ["items.json"])
        for file in (try? FileManager.default.contentsOfDirectory(atPath: folder.path(percentEncoded: false))) ?? [] where !kept.contains(file) {
            try? FileManager.default.removeItem(at: folder.appending(path: file))
        }
    }

    /// [item]'s image file, when it has one.
    static func image(of item: TopShelfItem) -> URL? {
        item.image.flatMap { folder?.appending(path: $0) }
    }
}

/// What a Top Shelf item opens in the app: its work's page, or its play.
enum TopShelfLink: Equatable {
    case work(String)
    case play(String)

    /// `kivo://work/{id}` or `kivo://play/{id}`; nil for any other link.
    init?(_ url: URL) {
        guard url.scheme == "kivo", let id = url.pathComponents.dropFirst().first else { return nil }
        switch url.host() {
        case "work": self = .work(id)
        case "play": self = .play(id)
        default: return nil
        }
    }

    var url: URL {
        switch self {
        case let .work(id): URL(string: "kivo://work/\(id)")!
        case let .play(id): URL(string: "kivo://play/\(id)")!
        }
    }
}
