import SwiftUI

/// Progress along the bottom edge of a card or an episode's still.
public struct ProgressBar: View {
    let value: Double
    @Environment(\.kivoPalette) private var palette

    public init(value: Double) {
        self.value = value
    }

    public var body: some View {
        GeometryReader { geometry in
            ZStack(alignment: .leading) {
                palette.progressTrack
                palette.progress.frame(width: geometry.size.width * value)
            }
        }
        .frame(height: 4)
        .accessibilityLabel(Text("\(Int(value * 100))% watched"))
    }
}
