import SwiftUI

/// A row to choose: its title, an optional second line, and a checkmark when it's the current one.
struct ChoiceRow: View {
    let title: String
    let detail: String?
    let chosen: Bool
    let choose: () -> Void

    var body: some View {
        Button(action: choose) {
            HStack {
                VStack(alignment: .leading, spacing: 2) {
                    Text(title)
                        .foregroundStyle(.primary)
                    if let detail {
                        Text(detail)
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                }
                Spacer()
                if chosen {
                    Image(systemName: "checkmark")
                        .font(.body.weight(.semibold))
                        .foregroundStyle(.tint)
                }
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityAddTraits(chosen ? .isSelected : [])
        .sensoryFeedback(.selection, trigger: chosen) { _, new in new }
    }
}
