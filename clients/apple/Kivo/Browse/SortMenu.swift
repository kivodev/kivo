import KivoUI
import SwiftUI

/// What a sort menu sorts by, with the sort in a word or two for the count line.
protocol SortMenuKey: Hashable {
    /// "A–Z", "Recently added"; nil to show the sort's label.
    var short: String? { get }
}

/// The sort in a word or two ("A–Z", "Recently added"), which opens a menu
/// of [sorts], each with the direction it sorts in.
struct SortMenu<Key: SortMenuKey>: View {
    let sorts: [GridModel.Sort<Key>]
    @Binding var selection: GridModel.Sort<Key>

    var body: some View {
        Menu {
            Picker(selection: $selection) {
                ForEach(sorts) { sort in
                    Button {} label: {
                        Text(sort.label)
                        if !sort.direction.isEmpty { Text(sort.direction) }
                    }
                    .tag(sort)
                }
            } label: {
                Text("Sort by")
            }
            .pickerStyle(.inline)
        } label: {
            HStack(spacing: 3) {
                Text(selection.short)
                Image(systemName: "chevron.up.chevron.down").font(.caption2.weight(.semibold))
            }
            .frame(minHeight: 32)
            .contentShape(Rectangle())
        }
        .accessibilityIdentifier("count-sort")
    }
}

/// A count line, "1,996 movies · A–Z": on one line when it fits, else the
/// sort under the count, so the sort's word always shows whole.
struct CountAndSort<Key: SortMenuKey>: View {
    let count: String
    let sorts: [GridModel.Sort<Key>]
    @Binding var selection: GridModel.Sort<Key>

    var body: some View {
        // At its ideal width: offered less, the menu draws its word narrower than it measures and cuts it short.
        ViewThatFits(in: .horizontal) {
            HStack(spacing: KivoTokens.spacing * 1.5) {
                Text(count)
                Text("·")
                SortMenu(sorts: sorts, selection: $selection).fixedSize()
            }
            VStack(alignment: .leading, spacing: 0) {
                Text(count)
                SortMenu(sorts: sorts, selection: $selection)
            }
        }
    }
}
