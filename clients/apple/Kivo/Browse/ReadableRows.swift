import KivoUI
import SwiftUI

extension View {
    /// In a wide window, keeps a list's rows to a readable width from the
    /// leading edge, as a detail page's text is, rather than across the window.
    func readableRows() -> some View {
        modifier(ReadableRows())
    }
}

private struct ReadableRows: ViewModifier {
    @Environment(\.wide) private var wide
    @State private var width: CGFloat = 0

    func body(content: Content) -> some View {
        content
            .contentMargins(.trailing, wide ? max(0, width - KivoTokens.Container.prose) : 0, for: .scrollContent)
            .onGeometryChange(for: CGFloat.self) { $0.size.width } action: { width = $0 }
    }
}
