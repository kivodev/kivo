import SwiftUI

/// An empty or failed state: a one-line title, one sentence of explanation
/// and a primary action, and at most one more, so no screen is a dead end.
public struct MessageView: View {
    public typealias Action = (label: String, run: () -> Void)

    private let title: String
    private let sentence: String?
    private let action: Action?
    private let secondary: Action?
    @Environment(\.kivoPalette) private var palette

    public init(_ title: String, sentence: String? = nil, action: Action? = nil, secondary: Action? = nil) {
        self.title = title
        self.sentence = sentence
        self.action = action
        self.secondary = secondary
    }

    public var body: some View {
        VStack(spacing: KivoTokens.spacing * 3) {
            Text(title)
                .kivoStyle(KivoTokens.TextSize.xl, weight: KivoTokens.Weight.semibold)
                .foregroundStyle(palette.text)
            if let sentence {
                Text(sentence)
                    .kivoStyle(KivoTokens.TextSize.base)
                    .foregroundStyle(palette.textMuted)
            }
            if let action {
                Button(action.label, action: action.run)
                    .buttonStyle(.borderedProminent)
            }
            if let secondary {
                Button(secondary.label, action: secondary.run)
                    .buttonStyle(.bordered)
            }
        }
        .multilineTextAlignment(.center)
        .frame(maxWidth: KivoTokens.Container.form)
        .padding(KivoTokens.spacing * 6)
    }
}
