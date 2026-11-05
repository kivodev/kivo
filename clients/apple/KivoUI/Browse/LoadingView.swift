import SwiftUI

/// A spinner that says what it waits for, as no spinner goes without a label.
public struct LoadingView: View {
    private let label: String
    @Environment(\.kivoPalette) private var palette

    public init(_ label: String) {
        self.label = label
    }

    public var body: some View {
        VStack(spacing: KivoTokens.spacing * 3) {
            ProgressView()
            Text(label)
                .kivoStyle(KivoTokens.TextSize.sm)
                .foregroundStyle(palette.textMuted)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}
