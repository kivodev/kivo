import KivoCore
import SwiftUI

/// The card of a library's collection, author or book series: its picture,
/// its name, and how many works it holds. An author's photo is round, a
/// book series' first cover sits on a stack, and a collection is its
/// poster, or a poster of its name when it has none.
public struct GroupCard: View {
    private enum Look { case cover, person, stack }

    private let name: String
    private let image: ImageRef?
    private let count: String
    private let look: Look
    private let style: CardStyle
    private let width: CGFloat
    private let caption: CardCaption
    @Environment(\.kivoPalette) private var palette

    /// A movie collection counts its movies; a videos library's folder, what it holds.
    public init(_ collection: CollectionSummary, width: CGFloat, caption: CardCaption = .twoLines) {
        let n = Int(collection.workCount)
        name = collection.name
        image = collection.images.poster
        count = collection.source == .provider
            ? (n == 1 ? String(localized: "1 movie") : String(localized: "\(n) movies"))
            : (n == 1 ? String(localized: "1 item") : String(localized: "\(n) items"))
        look = .cover
        style = .poster
        self.width = width
        self.caption = caption
    }

    public init(author: PersonSummary, width: CGFloat, caption: CardCaption = .twoLines) {
        name = author.name
        image = author.photo
        count = Self.books(Int(author.workCount))
        look = .person
        style = .square
        self.width = width
        self.caption = caption
    }

    public init(_ series: BookSeriesSummary, width: CGFloat, caption: CardCaption = .twoLines) {
        name = series.name
        image = series.cover
        count = Self.books(Int(series.bookCount))
        look = .stack
        style = .square
        self.width = width
        self.caption = caption
    }

    private static func books(_ n: Int) -> String {
        n == 1 ? String(localized: "1 book") : String(localized: "\(n) books")
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing) {
            if look == .person {
                PersonPhoto(image, name: name, size: width)
                    .cardLift()
            } else {
                ArtworkImage(image, title: name, pointWidth: width)
                    .frame(width: width, height: width / style.aspect)
                    .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
                    .background {
                        RoundedRectangle(cornerRadius: KivoTokens.Radius.md)
                            .fill(palette.placeholder)
                            .kivoShadow(palette.cardShadow)
                    }
                    .background(alignment: .top) {
                        if look == .stack { stack }
                    }
                    .cardLift()
            }
            switch caption {
            case .twoLines:
                // A person's name sits centred under their round photo.
                let person = look == .person
                VStack(alignment: person ? .center : .leading, spacing: 0) {
                    Text(name)
                        .kivoStyle(KivoTokens.TextSize.sm, weight: KivoTokens.Weight.medium)
                        .foregroundStyle(palette.text)
                        .lineLimit(1)
                    Text(count)
                        .kivoStyle(KivoTokens.TextSize.xs)
                        .foregroundStyle(palette.textMuted)
                        .lineLimit(1, reservesSpace: true)
                }
                .frame(width: width, alignment: person ? .center : .leading)
            case .oneLine:
                Text(name)
                    .kivoStyle(KivoTokens.TextSize.xs, weight: KivoTokens.Weight.medium)
                    .foregroundStyle(palette.text)
                    .lineLimit(1)
                    .frame(width: width, alignment: .leading)
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text("\(name), \(count)"))
    }

    /// A series' cover sits on two more, peeking above it, as a stack of books.
    private var stack: some View {
        ZStack(alignment: .top) {
            RoundedRectangle(cornerRadius: KivoTokens.Radius.md)
                .fill(palette.surfaceHover)
                .frame(width: width * 0.84, height: width / style.aspect)
                .offset(y: -8)
            RoundedRectangle(cornerRadius: KivoTokens.Radius.md)
                .fill(palette.border)
                .frame(width: width * 0.92, height: width / style.aspect)
                .offset(y: -4)
        }
        .kivoShadow(palette.cardShadow)
    }
}
