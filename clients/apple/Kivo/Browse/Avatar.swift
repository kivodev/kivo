import KivoUI
import SwiftUI

/// The user's avatar: their initial on the accent colour, both growing with the text size.
struct Avatar: View {
    let initial: String?
    @Environment(\.kivoPalette) private var palette
    @ScaledMetric private var size: CGFloat

    init(initial: String?, size: CGFloat = 32) {
        self.initial = initial
        _size = ScaledMetric(wrappedValue: size, relativeTo: .body)
    }

    var body: some View {
        Text(initial ?? "")
            .font(.system(size: size * 15 / 32, weight: .bold, design: .rounded))
            .foregroundStyle(palette.onAccent)
            .frame(width: size, height: size)
            .background(palette.accent, in: Circle())
    }
}
