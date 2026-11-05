import SwiftUI

public extension Animation {
    /// Content arriving: a short fade, easing out.
    static let kivoFade = Animation.easeOut(duration: 0.2)
}

private struct Fade<Value: Equatable>: ViewModifier {
    let value: Value
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    func body(content: Content) -> some View {
        content.animation(reduceMotion ? nil : .kivoFade, value: value)
    }
}

public extension View {
    /// Fades what comes and goes as [value] changes, unless Reduce Motion is on.
    func fades(with value: some Equatable) -> some View {
        modifier(Fade(value: value))
    }
}
