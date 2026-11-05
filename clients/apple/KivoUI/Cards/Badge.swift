import SwiftUI

/// A short label on artwork, with text, never colour alone.
public struct Badge: View {
    let text: String
    @Environment(\.kivoPalette) private var palette

    public init(text: String) {
        self.text = text
    }

    public var body: some View {
        Text(text)
            .kivoStyle(KivoTokens.TextSize.xs, weight: KivoTokens.Weight.semibold)
            .foregroundStyle(palette.onBadge)
            .padding(.horizontal, KivoTokens.spacing * 1.5)
            .padding(.vertical, KivoTokens.spacing * 0.5)
            .background(palette.badge, in: RoundedRectangle(cornerRadius: KivoTokens.Radius.sm))
    }
}
