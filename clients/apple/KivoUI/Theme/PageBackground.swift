import SwiftUI

public extension View {
    /// Gives a page of a navigation stack the palette's background as the
    /// stack's own, so it fills the screen under the bars in every state the
    /// page shows, loading and unreachable included, and through a push or
    /// pop. A page without it shows the system's background.
    func kivoPageBackground() -> some View {
        modifier(PageBackground())
    }
}

private struct PageBackground: ViewModifier {
    @Environment(\.kivoPalette) private var palette

    func body(content: Content) -> some View {
        content.containerBackground(palette.bg, for: .navigation)
    }
}
