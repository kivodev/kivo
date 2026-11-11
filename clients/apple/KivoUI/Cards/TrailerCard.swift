import KivoCore
import SwiftUI

/// A trailer on YouTube: its thumbnail, marked as leaving for YouTube
/// before it's opened, then its title. With no thumbnail the tile is bare,
/// as the title is under it.
public struct TrailerCard: View {
    private let trailer: Trailer
    private let width: CGFloat
    @Environment(\.kivoPalette) private var palette

    public init(_ trailer: Trailer, width: CGFloat) {
        self.trailer = trailer
        self.width = width
    }

    public var body: some View {
        let opens = String(localized: "Opens in YouTube")
        VStack(alignment: .leading, spacing: KivoTokens.spacing) {
            ZStack(alignment: .bottomLeading) {
                ArtworkImage(trailer.image, title: "", pointWidth: width)
                Badge(text: "YouTube ↗")
                    .padding(KivoTokens.spacing * 1.5)
            }
            .frame(width: width, height: width / CardStyle.landscape.aspect)
            .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
            .background {
                RoundedRectangle(cornerRadius: KivoTokens.Radius.md)
                    .fill(palette.placeholder)
                    .kivoShadow(palette.cardShadow)
            }

            Text(trailer.title)
                .kivoStyle(KivoTokens.TextSize.sm, weight: KivoTokens.Weight.medium)
                .foregroundStyle(palette.text)
                .lineLimit(1)
                .frame(width: width, alignment: .leading)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text(verbatim: "\(trailer.title), \(opens)"))
    }
}
