import SwiftUI

// The theme in SwiftUI: the palette of tokens.json for the
// device's Theme setting, in the environment, and the type scale as fonts.
// Dynamic Type scales every size from the token's.

private struct KivoPaletteKey: EnvironmentKey {
    static let defaultValue = KivoPalette.dark
}

public extension EnvironmentValues {
    /// The colors of this device's theme.
    var kivoPalette: KivoPalette {
        get { self[KivoPaletteKey.self] }
        set { self[KivoPaletteKey.self] = newValue }
    }
}

public extension View {
    /// Applies a palette: its colors to the views below and, unless it
    /// follows the system's appearance, its scheme to the system's.
    func kivoTheme(_ palette: KivoPalette, followsSystem: Bool = false) -> some View {
        environment(\.kivoPalette, palette)
            .preferredColorScheme(followsSystem ? nil : palette.scheme)
            .tint(palette.accent)
    }

    /// Gives a page of a navigation stack the palette's background as the
    /// stack's own, so it fills the screen under the bars in every state the
    /// page shows, loading and unreachable included, and through a push or
    /// pop. A page without it shows the system's background.
    func kivoPageBackground() -> some View {
        modifier(PageBackground())
    }

    /// The card shadow, or any of the palette's layered shadows.
    func kivoShadow(_ layers: [KivoShadow]) -> some View {
        layers.reduce(AnyView(self)) { view, layer in
            AnyView(view.shadow(color: layer.color, radius: layer.radius, x: layer.x, y: layer.y))
        }
    }
}

private struct PageBackground: ViewModifier {
    @Environment(\.kivoPalette) private var palette

    func body(content: Content) -> some View {
        content.containerBackground(palette.bg, for: .navigation)
    }
}

public extension Font {
    /// A size of the type scale, scaled with Dynamic Type like the text style nearest it.
    static func kivo(_ style: KivoTextStyle, weight: Font.Weight = KivoTokens.Weight.normal) -> Font {
        .system(size: style.size, weight: weight)
    }
}

/// A Dynamic Type aware size of the type scale, for use as `@ScaledMetric`'s base.
public extension KivoTextStyle {
    var relativeTextStyle: Font.TextStyle {
        switch size {
        case ..<13: .caption
        case ..<15: .footnote
        case ..<17: .body
        case ..<20: .title3
        case ..<26: .title2
        case ..<32: .title
        default: .largeTitle
        }
    }
}

public extension Text {
    /// Text in a size of the type scale that follows Dynamic Type.
    func kivoStyle(_ style: KivoTextStyle, weight: Font.Weight = KivoTokens.Weight.normal) -> some View {
        modifier(KivoTextModifier(style: style, weight: weight))
    }
}

private struct KivoTextModifier: ViewModifier {
    let style: KivoTextStyle
    let weight: Font.Weight
    @ScaledMetric private var scale: CGFloat = 1

    init(style: KivoTextStyle, weight: Font.Weight) {
        self.style = style
        self.weight = weight
        _scale = ScaledMetric(wrappedValue: 1, relativeTo: style.relativeTextStyle)
    }

    func body(content: Content) -> some View {
        content
            .font(.system(size: style.size * scale, weight: weight))
            .lineSpacing(max(0, (style.lineHeight - style.size) * scale / 2))
    }
}
