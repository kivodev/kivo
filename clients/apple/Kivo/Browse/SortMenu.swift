import SwiftUI

/// The sort in a word or two ("A–Z", "Recently added"), which opens a menu
/// of [sorts], each with the direction it sorts in.
struct SortMenu: View {
    let sorts: [GridModel.Sort]
    @Binding var selection: GridModel.Sort

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
