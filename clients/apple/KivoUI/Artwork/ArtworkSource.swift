import KivoCore
import Nuke
import NukeUI
import SwiftUI

/// Where artwork comes from: the server's image route (`GET
/// /api/v1/images/{id}?width=`) at the address in use, with the session's
/// `Authorization` header, since images are only for a signed-in viewer,
/// when [authorization] gives it for the image's URL: only at an address
/// the session's token may go to now.
/// An image is cached by its server and ID, not its URL, so it stays
/// cached as the server's address changes.
public struct ArtworkSource: Sendable {
    public let baseURL: String
    /// The `Authorization` header for a URL, nil where the token may not go now.
    public let authorization: @Sendable (String) -> String?
    public let serverId: String

    public init(baseURL: String, authorization: @escaping @Sendable (String) -> String?, serverId: String) {
        self.baseURL = baseURL
        self.authorization = authorization
        self.serverId = serverId
    }

    /// The request for image [id] at least [pixelWidth] wide. Widths are
    /// rounded up to a step, so cards of nearby sizes share a cached image.
    func request(id: String, pixelWidth: CGFloat) -> ImageRequest? {
        let width = Int((pixelWidth / 160).rounded(.up) * 160)
        guard let url = URL(string: "\(baseURL)/api/v1/images/\(id)?width=\(max(160, width))") else { return nil }
        var request = URLRequest(url: url)
        if let authorization = authorization(url.absoluteString) { request.setValue(authorization, forHTTPHeaderField: "Authorization") }
        var image = ImageRequest(urlRequest: request)
        image.imageID = "\(serverId)/\(id)/\(max(160, width))"
        return image
    }

    /// Image [id] at least [pixelWidth] wide, through the same caches as the
    /// views, for what draws it outside SwiftUI, such as Now Playing.
    public func image(id: String, pixelWidth: CGFloat) async -> UIImage? {
        guard let request = request(id: id, pixelWidth: pixelWidth) else { return nil }
        return try? await ImagePipeline.shared.image(for: request)
    }

    /// Sets up artwork's caches, once as the app starts: decoded images in
    /// memory (Nuke's default, a share of the device's memory), and the
    /// server's images on disk, used without asking the server again, as an
    /// image by its ID never changes. URLSession's own cache is off.
    public static func configureCaches() {
        ImagePipeline.shared = ImagePipeline(configuration: .withDataCache(name: "com.kivo.player.artwork", sizeLimit: 500 * 1024 * 1024))
    }
}

private struct ArtworkSourceKey: EnvironmentKey {
    static let defaultValue: ArtworkSource? = nil
}

public extension EnvironmentValues {
    var artworkSource: ArtworkSource? {
        get { self[ArtworkSourceKey.self] }
        set { self[ArtworkSourceKey.self] = newValue }
    }
}
