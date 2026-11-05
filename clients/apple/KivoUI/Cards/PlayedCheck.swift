import SwiftUI

/// The played mark.
public struct PlayedCheck: View {
    @Environment(\.kivoPalette) private var palette

    public init() {}

    public var body: some View {
        Image(systemName: "checkmark")
            .font(.caption.weight(.bold))
            .foregroundStyle(palette.onBadge)
            .padding(KivoTokens.spacing)
            .background(palette.badge, in: Circle())
            .accessibilityLabel(Text("Played"))
    }
}
