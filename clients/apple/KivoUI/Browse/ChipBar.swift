import SwiftUI

/// One chip of a ``ChipBar``.
public struct Chip: Identifiable, Hashable {
    public let id: String
    public let label: String
    public let selected: Bool

    public init(id: String, label: String, selected: Bool) {
        self.id = id
        self.label = label
        self.selected = selected
    }
}

/// A row of small chips that scrolls sideways: a library's
/// tabs and quick filters. A selected chip takes the accent colour; the
/// rest stay neutral.
public struct ChipBar: View {
    private let chips: [Chip]
    private let choose: (Chip) -> Void
    @Environment(\.kivoPalette) private var palette

    public init(_ chips: [Chip], choose: @escaping (Chip) -> Void) {
        self.chips = chips
        self.choose = choose
    }

    public var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: KivoTokens.spacing * 2) {
                ForEach(chips) { chip in
                    Button {
                        choose(chip)
                    } label: {
                        ChipLabel(label: chip.label)
                            .foregroundStyle(chip.selected ? palette.onAccent : palette.text)
                            .background(chip.selected ? palette.accent : palette.surfaceRaised, in: Capsule())
                            .contentShape(Capsule())
                    }
                    .buttonStyle(.plain)
                    .accessibilityAddTraits(chip.selected ? .isSelected : [])
                    .accessibilityIdentifier("chip-\(chip.id)")
                }
            }
            .padding(.horizontal, KivoTokens.spacing * 4)
        }
        .scrollClipDisabled()
        .sensoryFeedback(.selection, trigger: chips.filter(\.selected).map(\.id))
    }
}

/// A chip's label, at the size every chip has.
private struct ChipLabel: View {
    let label: String

    var body: some View {
        Text(label)
            .font(.subheadline.weight(.medium))
            .padding(.horizontal, KivoTokens.spacing * 3.5)
            .frame(minHeight: 32)
    }
}

/// A ``ChipBar``'s skeleton: a capsule the size of each chip of [labels].
public struct SkeletonChipBar: View {
    private let labels: [String]

    public init(labels: [String]) {
        self.labels = labels
    }

    public var body: some View {
        HStack(spacing: KivoTokens.spacing * 2) {
            ForEach(labels, id: \.self) { label in
                ChipLabel(label: label).skeleton(Capsule())
            }
        }
        .padding(.horizontal, KivoTokens.spacing * 4)
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
