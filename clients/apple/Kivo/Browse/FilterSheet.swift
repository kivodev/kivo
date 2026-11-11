import KivoCore
import KivoUI
import SwiftUI

// FilterStyleTrial: the filter sheet, one of the two styles being compared.

/// The Filter button, opening the filter sheet. Save as Smart List, chosen
/// in the sheet, opens once the sheet has gone.
struct FilterSheetButton: View {
    let model: GridModel
    @Binding var savingAsSmartList: Bool
    @State private var open = false
    @State private var saving = false
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Button {
            open = true
        } label: {
            FilterLabel(on: !model.state.filters.isEmpty)
        }
        .tint(palette.text)
        .accessibilityIdentifier("filter")
        .sheet(isPresented: $open, onDismiss: {
            if saving {
                saving = false
                savingAsSmartList = true
            }
        }) {
            FilterSheet(model: model) { saving = true }
        }
    }
}

/// Filters: watch status, then a group of chips for each filter the library
/// offers, each shown only when it has something: decade, genre, content
/// rating and quality, or a books library's decade, genre and narrator.
/// The grid behind follows the choices once they've held still, and Show
/// gives its count, the last one known while the next is read.
struct FilterSheet: View {
    let model: GridModel
    let saveAsSmartList: () -> Void
    /// The grid's count, the last one known while the next is read.
    @State private var count: Int?
    /// The long groups opened whole.
    @State private var whole: Set<FilterGroupKind> = []
    @State private var detent = PresentationDetent.large
    @Environment(\.dismiss) private var dismiss
    @Environment(\.kivoPalette) private var palette
    @Environment(\.dynamicTypeSize) private var typeSize

    /// All, then each watch status.
    private static let statuses: [WorkPlayState?] = [nil, .unplayed, .inProgress, .played]

    init(model: GridModel, saveAsSmartList: @escaping () -> Void) {
        self.model = model
        self.saveAsSmartList = saveAsSmartList
        _count = State(initialValue: model.paged.total)
    }

    /// The filters chosen, which reach the grid once they've held still.
    private var chosen: GridFilters { model.state.filters }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: KivoTokens.spacing * 6) {
                    ForEach(model.state.offer, id: \.kind) { group in
                        if group.kind == .watchStatus {
                            self.group(GridModel.title(group.kind)) { watchStatus }
                        } else {
                            self.group(GridModel.title(group.kind), count: group.values.count, whole: group.isLong ? opened(group.kind) : nil) {
                                ForEach(group.shown(whole: whole.contains(group.kind), chosen: chosen), id: \.key) { value in
                                    FilterChip(label: model.label(value), selected: chosen.has(filter: value)) {
                                        model.toggle(value, settle: true)
                                    }
                                }
                            }
                        }
                    }
                    if !chosen.isEmpty {
                        Button(String(localized: "Save as Smart List"), systemImage: "sparkles") {
                            saveAsSmartList()
                            dismiss()
                        }
                        .font(.subheadline.weight(.medium))
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, KivoTokens.spacing * 5)
                .padding(.top, KivoTokens.spacing * 2)
                .padding(.bottom, KivoTokens.spacing * 4)
            }
            .safeAreaInset(edge: .bottom, spacing: 0) { footer }
            .navigationTitle(String(localized: "Filters"))
            .toolbarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(String(localized: "Clear")) { model.clearFilters(settle: true) }
                        .disabled(chosen.isEmpty)
                        .accessibilityIdentifier("filters-clear")
                }
            }
            .containerBackground(palette.surface, for: .navigation)
        }
        .presentationDetents([.medium, .large], selection: $detent)
        .presentationDragIndicator(.visible)
        .presentationBackground(palette.surface)
        .onChange(of: model.paged.total) {
            if let total = model.paged.total { count = total }
        }
        // The grid shows what's chosen as the sheet closes.
        .onDisappear { model.choose(chosen) }
    }

    /// Whether [group] is opened whole, as its All or Fewer button sets it.
    private func opened(_ group: FilterGroupKind) -> Binding<Bool> {
        Binding(
            get: { whole.contains(group) },
            set: { open in
                if open { whole.insert(group) } else { whole.remove(group) }
            }
        )
    }

    /// The watch status chosen, All being none.
    private var played: Binding<WorkPlayState?> {
        Binding(
            get: { chosen.played },
            set: { played in
                let next = played.map { chosen.choosing(filter: GridFilter.Played(state: $0)) } ?? chosen.clearing(group: .watchStatus)
                model.choose(next, settle: true)
            }
        )
    }

    /// All, Unwatched, In progress, Watched: a segmented control, or chips
    /// at the largest text sizes, where the segments would cut their words.
    @ViewBuilder private var watchStatus: some View {
        if typeSize.isAccessibilitySize {
            ForEach(Self.statuses, id: \.self) { status in
                FilterChip(label: model.label(played: status), selected: chosen.played == status) { played.wrappedValue = status }
            }
        } else {
            Picker(String(localized: "Watch status"), selection: played) {
                ForEach(Self.statuses, id: \.self) { Text(model.label(played: $0)).tag($0) }
            }
            .pickerStyle(.segmented)
            .frame(maxWidth: .infinity)
        }
    }

    /// A group: its title, with All or Fewer when it's long, over its chips.
    private func group(
        _ title: String,
        count: Int = 0,
        whole: Binding<Bool>? = nil,
        @ViewBuilder chips: () -> some View
    ) -> some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing * 2.5) {
            HStack(alignment: .firstTextBaseline) {
                Text(title)
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(palette.textMuted)
                    .accessibilityAddTraits(.isHeader)
                Spacer()
                if let whole {
                    Button(whole.wrappedValue ? String(localized: "Fewer") : String(localized: "All \(count)")) {
                        withAnimation(.snappy) { whole.wrappedValue.toggle() }
                    }
                    .font(.subheadline.weight(.medium))
                }
            }
            FlowLayout(spacing: KivoTokens.spacing * 2) {
                chips()
            }
        }
    }

    /// Show 41 movies, pinned under the groups, which closes the sheet.
    private var footer: some View {
        Button {
            dismiss()
        } label: {
            Text(count.map { String(localized: "Show \(model.noun($0))") } ?? String(localized: "Show"))
                .font(.headline)
                .foregroundStyle(palette.onAccent)
                .frame(maxWidth: .infinity, minHeight: 34)
                .contentTransition(.numericText())
                .animation(.snappy, value: count)
        }
        .buttonStyle(.borderedProminent)
        .buttonBorderShape(.roundedRectangle(radius: KivoTokens.Radius.lg))
        .controlSize(.large)
        .padding(.horizontal, KivoTokens.spacing * 5)
        .padding(.top, KivoTokens.spacing * 3)
        .padding(.bottom, KivoTokens.spacing * 2)
        .background(palette.surface)
        .overlay(alignment: .top) {
            Rectangle().fill(palette.border).frame(height: 1 / 3)
        }
        .accessibilityIdentifier("filters-show")
    }
}

/// A choice in the filter sheet: neutral, or the accent colour while chosen.
private struct FilterChip: View {
    let label: String
    let selected: Bool
    let toggle: () -> Void
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        Button(action: toggle) {
            Text(label)
                .font(.subheadline.weight(.medium))
                .multilineTextAlignment(.leading)
                .foregroundStyle(selected ? palette.onAccent : palette.text)
                .padding(.horizontal, KivoTokens.spacing * 3.5)
                .padding(.vertical, KivoTokens.spacing * 2)
                .frame(minHeight: 44)
                .background(selected ? palette.accent : palette.surfaceHover, in: RoundedRectangle(cornerRadius: 22))
                .contentShape(RoundedRectangle(cornerRadius: 22))
        }
        .buttonStyle(.plain)
        .accessibilityAddTraits(selected ? .isSelected : [])
        .sensoryFeedback(.selection, trigger: selected)
    }
}

/// Lays its views out in rows, starting a new row where the next doesn't fit.
private struct FlowLayout: Layout {
    let spacing: CGFloat

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let width = proposal.width ?? .infinity
        let rows = rows(of: subviews, width: width)
        let height = rows.map(\.height).reduce(0, +) + spacing * CGFloat(max(rows.count - 1, 0))
        let widest = rows.map(\.width).max() ?? 0
        return CGSize(width: proposal.width ?? widest, height: height)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var y = bounds.minY
        for row in rows(of: subviews, width: bounds.width) {
            var x = bounds.minX
            for (index, size) in zip(row.indices, row.sizes) {
                subviews[index].place(at: CGPoint(x: x, y: y), proposal: ProposedViewSize(size))
                x += size.width + spacing
            }
            y += row.height + spacing
        }
    }

    private struct Row {
        var indices: [Int] = []
        var sizes: [CGSize] = []
        var width: CGFloat = 0
        var height: CGFloat = 0
    }

    private func rows(of subviews: Subviews, width: CGFloat) -> [Row] {
        var rows: [Row] = []
        var row = Row()
        for (index, subview) in subviews.enumerated() {
            let size = subview.sizeThatFits(ProposedViewSize(width: width, height: nil))
            let next = row.indices.isEmpty ? size.width : row.width + spacing + size.width
            if next > width, !row.indices.isEmpty {
                rows.append(row)
                row = Row()
            }
            row.width = row.indices.isEmpty ? size.width : row.width + spacing + size.width
            row.height = max(row.height, size.height)
            row.indices.append(index)
            row.sizes.append(size)
        }
        if !row.indices.isEmpty { rows.append(row) }
        return rows
    }
}
