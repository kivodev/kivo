import KivoCore
import KivoUI
import SwiftUI

/// A card that opens its work's page, with a long press's menu: Play or
/// Resume, Mark as Watched, Favorite, Go to Series for an episode, and Go
/// to Library where the card isn't already in that library's grid. A
/// pointer over it highlights it.
struct WorkLink<Label: View>: View {
    let work: WorkSummary
    @ViewBuilder let label: () -> Label
    @Environment(\.openWork) private var openWork

    var body: some View {
        // A button that pushes, rather than a NavigationLink, which a list would mark with a chevron.
        Button {
            openWork(WorkRoute(work))
        } label: {
            label()
        }
        .buttonStyle(.plain)
        .hoverEffect(.highlight)
        .accessibilityIdentifier("work")
        .contextMenu { WorkActions(work: work) }
    }
}
