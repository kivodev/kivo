import SwiftUI

/// Placeholder shapes in the layout of what's loading, so a screen keeps
/// its shape while it waits rather than showing grey space. They pulse
/// gently unless Reduce Motion is on.
public struct SkeletonBlock: View {
    private let width: CGFloat?
    private let height: CGFloat?
    private let radius: CGFloat
    @Environment(\.kivoPalette) private var palette

    /// A nil [width] or [height] fills what the block is given.
    public init(width: CGFloat? = nil, height: CGFloat? = nil, radius: CGFloat = KivoTokens.Radius.md) {
        self.width = width
        self.height = height
        self.radius = radius
    }

    public var body: some View {
        RoundedRectangle(cornerRadius: radius, style: .continuous)
            .fill(palette.placeholder)
            .frame(width: width, height: height)
            .pulsing()
            .accessibilityHidden(true)
    }
}

private struct SkeletonShape<S: Shape>: ViewModifier {
    let shape: S
    @Environment(\.kivoPalette) private var palette

    func body(content: Content) -> some View {
        content
            .hidden()
            .overlay { shape.fill(palette.placeholder).pulsing() }
            .accessibilityHidden(true)
    }
}

/// A line of text's place: a bar about as tall as the letters, centred on
/// the line, [width] wide or as wide as the line.
private struct SkeletonLine: ViewModifier {
    let width: CGFloat?
    let alignment: Alignment

    func body(content: Content) -> some View {
        content
            .hidden()
            .frame(width: width)
            .frame(maxWidth: width == nil ? .infinity : nil, alignment: alignment)
            .overlay {
                GeometryReader { geometry in
                    SkeletonBlock(width: geometry.size.width, height: (geometry.size.height * 0.6).rounded(), radius: 4)
                        .frame(maxHeight: .infinity)
                }
            }
            .accessibilityHidden(true)
    }
}

public extension View {
    /// This view's place, filled with [shape]: the view lays out as it
    /// would, so its skeleton is its size, but doesn't draw.
    func skeleton(_ shape: some Shape) -> some View {
        modifier(SkeletonShape(shape: shape))
    }

    /// A line of text's place: lay out a blank line in the text's style and
    /// it takes the line's height, with a bar [width] wide, or as wide as
    /// the line is given.
    func skeletonLine(width: CGFloat? = nil, alignment: Alignment = .leading) -> some View {
        modifier(SkeletonLine(width: width, alignment: alignment))
    }
}

/// A card's skeleton: the artwork's shape and bars for its caption, as
/// tall as the card with [caption], so the card takes its place without a jump.
public struct SkeletonCard: View {
    private let style: CardStyle
    private let width: CGFloat
    private let caption: CardCaption
    private let person: Bool

    /// A [person]'s card is their round photo.
    public init(style: CardStyle, width: CGFloat, caption: CardCaption = .twoLines, person: Bool = false) {
        self.style = style
        self.width = width
        self.caption = caption
        self.person = person
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing) {
            SkeletonBlock(width: width, height: width / style.aspect, radius: person ? width / 2 : KivoTokens.Radius.md)
            CaptionSkeleton(caption: caption, width: width)
        }
    }
}

/// A row of cards' skeleton: a heading and the cards a row starts with,
/// laid out as a row of cards is, so the row takes its place without a jump.
public struct SkeletonRow<Heading: View>: View {
    private let style: CardStyle
    private let width: CGFloat
    private let spacing: CGFloat
    private let inset: CGFloat
    private let gap: CGFloat
    private let heading: Heading

    /// [heading] is a blank line in the row's heading style; [spacing] is
    /// between the heading and the cards, [inset] the row's leading edge,
    /// and [gap] between its cards.
    public init(style: CardStyle, width: CGFloat, spacing: CGFloat, inset: CGFloat, gap: CGFloat = KivoTokens.spacing * 3, @ViewBuilder heading: () -> Heading) {
        self.style = style
        self.width = width
        self.spacing = spacing
        self.inset = inset
        self.gap = gap
        self.heading = heading()
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: spacing) {
            heading
                .skeletonLine(width: 160)
                .padding(.horizontal, inset)
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(alignment: .top, spacing: gap) {
                    ForEach(0..<6, id: \.self) { _ in SkeletonCard(style: style, width: width) }
                }
                .padding(.horizontal, inset)
            }
            .scrollDisabled(true)
        }
    }
}

private struct Pulse: ViewModifier {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    func body(content: Content) -> some View {
        if reduceMotion {
            content
        } else {
            content.phaseAnimator([1.0, 0.55]) { view, opacity in
                view.opacity(opacity)
            } animation: { _ in
                .easeInOut(duration: 0.9)
            }
        }
    }
}

public extension View {
    /// Fades in and out slowly, as a skeleton does while it waits.
    func pulsing() -> some View {
        modifier(Pulse())
    }
}
