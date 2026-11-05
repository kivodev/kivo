import KivoCore
import SwiftUI
import UIKit

/// The letter scrubber beside a grid sorted by title, a list's section
/// index for a grid: a strip of small letters on the trailing edge with a
/// hit area 44 points wide. Touching or dragging along it shows the letter
/// in a large bubble beside the finger, ticks on each new letter and jumps
/// the grid to that letter's first work as it goes.
/// Letters with no works aren't listed, so each one goes somewhere; # is
/// the top. Where the letters don't fit, every other one becomes a dot, and
/// the finger still reaches each letter. VoiceOver reads it as one
/// adjustable control.
public struct LetterScrubber: View {
    /// The width the scrubber takes on the trailing edge, its hit area.
    public static let width: CGFloat = 44
    /// The width of the visible letters, for the grid's trailing inset.
    public static let stripWidth: CGFloat = 20
    private static let rowHeight: CGFloat = 15
    private static let stripPadding: CGFloat = 6
    /// A navigation bar's height with its title collapsed.
    private static let compactBarHeight: CGFloat = 44

    /// The window's own top inset: the status bar, or the Dynamic Island's.
    @MainActor private static var windowTop: CGFloat {
        let scene = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first
        return scene?.keyWindow?.safeAreaInsets.top ?? 0
    }

    private let letters: [LetterStart]
    private let current: String?
    private let jump: (LetterStart) -> Void
    @Environment(\.kivoPalette) private var palette
    @State private var touched: Int?
    @State private var fingerY: CGFloat = 0
    /// Where the scrubber was on screen as a touch began, so it stays under
    /// the finger while the bar above it collapses or grows back.
    @State private var held: (top: CGFloat, height: CGFloat)?

    public init(letters: [LetterStart], current: String?, jump: @escaping (LetterStart) -> Void) {
        self.letters = letters
        self.current = current
        self.jump = jump
    }

    private enum Slot: Hashable {
        case letter(Int)
        case dot(Int)
    }

    public var body: some View {
        GeometryReader { geometry in
            let minY = geometry.frame(in: .global).minY
            let height = held?.height ?? geometry.size.height
            // Centred below where the bar ends once the title has collapsed,
            // which a title collapsing or growing back doesn't move; kept
            // clear of the bar while the title is large.
            let bar = geometry.safeAreaInsets.top
            let compact = min(height, Self.windowTop + Self.compactBarHeight)
            let fitting = Int((height - compact - Self.stripPadding * 2) / Self.rowHeight)
            let slots = Self.slots(count: letters.count, fitting: fitting)
            let stripHeight = CGFloat(slots.count) * Self.rowHeight + Self.stripPadding * 2
            let top = held.map { $0.top - minY } ?? max(bar, compact + (height - compact - stripHeight) / 2)
            strip(slots)
                .frame(width: Self.width, height: stripHeight, alignment: .trailing)
                .contentShape(Rectangle())
                .gesture(
                    DragGesture(minimumDistance: 0)
                        .onChanged { value in
                            if held == nil { held = (minY + top, height) }
                            touch(at: value.location.y, height: stripHeight)
                        }
                        .onEnded { _ in
                            touched = nil
                            held = nil
                        }
                )
                .offset(y: top)
                .frame(width: geometry.size.width, height: geometry.size.height, alignment: .topTrailing)
                // The bubble draws beside the strip, outside the scrubber's own width.
                .overlay(alignment: .topTrailing) {
                    if let touched {
                        bubble(letters[touched].letter)
                            .offset(x: -(Self.width + 4), y: min(max(0, top + fingerY - 32), geometry.size.height - 64))
                            .allowsHitTesting(false)
                            .transition(.opacity)
                    }
                }
                .animation(.easeOut(duration: 0.15), value: touched == nil)
        }
        .frame(width: Self.width)
        .sensoryFeedback(.selection, trigger: touched) { _, new in new != nil }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text("Jump to letter"))
        .accessibilityValue(Text(current ?? ""))
        .accessibilityIdentifier("scrubber")
        .accessibilityAdjustableAction { direction in
            let index = letters.firstIndex { $0.letter == current } ?? 0
            let next = direction == .increment ? index + 1 : index - 1
            if letters.indices.contains(next) { jump(letters[next]) }
        }
    }

    private func strip(_ slots: [Slot]) -> some View {
        let marked = touched.map { letters[$0].letter } ?? current
        return VStack(spacing: 0) {
            ForEach(slots, id: \.self) { slot in
                Group {
                    switch slot {
                    case let .letter(index):
                        Text(letters[index].letter)
                            .font(.system(size: 11, weight: .semibold))
                            .foregroundStyle(letters[index].letter == marked ? palette.accent : palette.textMuted)
                    case .dot:
                        Circle()
                            .fill(palette.textSubtle)
                            .frame(width: 4, height: 4)
                    }
                }
                .frame(width: Self.stripWidth, height: Self.rowHeight)
            }
        }
        .padding(.vertical, Self.stripPadding)
        .background {
            Capsule()
                .fill(palette.surfaceRaised.opacity(touched == nil ? 0 : 0.85))
        }
    }

    @ViewBuilder private func bubble(_ letter: String) -> some View {
        let shape = RoundedRectangle(cornerRadius: 18, style: .continuous)
        let text = Text(letter)
            .font(.system(size: 36, weight: .bold, design: .rounded))
            .foregroundStyle(palette.text)
            .frame(width: 64, height: 64)
        if #available(iOS 26.0, *) {
            text.glassEffect(.regular, in: shape)
        } else {
            text.background(.regularMaterial, in: shape)
        }
    }

    /// The letter under [y] in the strip: the strip's length is shared out
    /// over every letter, dotted or not.
    private func touch(at y: CGFloat, height: CGFloat) {
        guard !letters.isEmpty else { return }
        let inner = height - Self.stripPadding * 2
        fingerY = min(max(y, Self.stripPadding), height - Self.stripPadding)
        let fraction = (y - Self.stripPadding) / max(1, inner)
        let index = min(letters.count - 1, max(0, Int(fraction * CGFloat(letters.count))))
        guard index != touched else { return }
        touched = index
        jump(letters[index])
    }

    /// Every letter when they fit in [fitting] rows; otherwise evenly spread
    /// letters, the first and the last among them, with a dot between each.
    private static func slots(count: Int, fitting: Int) -> [Slot] {
        guard count > fitting, fitting >= 3 else { return (0..<count).map { .letter($0) } }
        let shown = (fitting + 1) / 2
        return (0..<shown).flatMap { k -> [Slot] in
            let index = Int((Double(k) * Double(count - 1) / Double(shown - 1)).rounded())
            return k == 0 ? [.letter(index)] : [.dot(k), .letter(index)]
        }
    }
}
