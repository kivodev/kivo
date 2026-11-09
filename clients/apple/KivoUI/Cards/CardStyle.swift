import KivoCore
import SwiftUI

/// The three card shapes.
public enum CardStyle: Sendable {
    /// 2:3, for movies, series, seasons and collections.
    case poster
    /// 16:9, for episodes and other videos, and in Continue Watching.
    case landscape
    /// 1:1, for books, book series and authors.
    case square

    /// The shape a work's card takes where nothing else decides it.
    public static func of(_ work: WorkSummary) -> CardStyle {
        switch work.kind {
        case "episode", "video", "extra": .landscape
        case "book": .square
        default: .poster
        }
    }

    /// The shape a work's card takes in a list: an episode's is a poster.
    public static func listed(_ work: WorkSummary) -> CardStyle {
        work.kind == "episode" ? .poster : of(work)
    }

    public var aspect: CGFloat {
        switch self {
        case .poster: KivoTokens.Aspect.poster
        case .landscape: KivoTokens.Aspect.landscape
        case .square: KivoTokens.Aspect.square
        }
    }

    /// The image a card of this shape shows for [work].
    public func artwork(of work: WorkSummary) -> ImageRef? {
        switch self {
        case .poster, .square: work.images.poster ?? work.images.still
        case .landscape: work.images.still ?? work.images.backdrop ?? work.images.poster
        }
    }

    /// The narrowest a card of this shape is in a grid.
    public var minWidth: CGFloat {
        switch self {
        case .poster, .square: KivoTokens.Layout.posterMinWidth * 0.75
        case .landscape: KivoTokens.Layout.landscapeMinWidth * 0.75
        }
    }
}
