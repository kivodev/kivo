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
            FilterLabel(on: model.query.filtered)
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
    /// The choices, which reach the grid once they've held still.
    @State private var draft: GridModel.Query
    /// The grid's count, the last one known while the next is read.
    @State private var count: Int?
    @State private var sending: Task<Void, Never>?
    @State private var allGenres = false
    @State private var allNarrators = false
    @State private var detent = PresentationDetent.large
    @Environment(\.dismiss) private var dismiss
    @Environment(\.kivoPalette) private var palette
    @Environment(\.dynamicTypeSize) private var typeSize

    /// How long the choices hold still before the grid follows them.
    private static let settleTime = Duration.milliseconds(300)
    /// A long group shows this many chips until it's opened whole.
    private static let shortLength = 9

    init(model: GridModel, saveAsSmartList: @escaping () -> Void) {
        self.model = model
        self.saveAsSmartList = saveAsSmartList
        _draft = State(initialValue: model.query)
        _count = State(initialValue: model.paged.total)
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: KivoTokens.spacing * 6) {
                    group(String(localized: "Watch status")) { watchStatus }
                    if !model.decades.isEmpty {
                        group(String(localized: "Decade")) {
                            ForEach(model.decades, id: \.self) { decade in
                                FilterChip(label: GridModel.label(decade: decade), selected: draft.decade == decade) {
                                    draft.decade = draft.decade == decade ? nil : decade
                                }
                            }
                        }
                    }
                    if !model.genres.isEmpty {
                        let genres = shortened(model.genres, whole: allGenres) { draft.genres.contains($0) }
                        group(String(localized: "Genre"), count: model.genres.count, whole: $allGenres) {
                            ForEach(genres, id: \.self) { genre in
                                FilterChip(label: genre, selected: draft.genres.contains(genre)) { draft.genres.toggle(genre) }
                            }
                        }
                    }
                    if !model.contentRatings.isEmpty {
                        group(String(localized: "Content rating")) {
                            ForEach(model.contentRatings, id: \.self) { rating in
                                FilterChip(label: rating, selected: draft.contentRatings.contains(rating)) { draft.contentRatings.toggle(rating) }
                            }
                        }
                    }
                    if !model.resolutions.isEmpty || !model.dynamicRanges.isEmpty {
                        group(String(localized: "Quality")) {
                            ForEach(model.resolutions, id: \.self) { resolution in
                                FilterChip(label: GridModel.label(resolution), selected: draft.resolutions.contains(resolution)) {
                                    draft.resolutions.toggle(resolution)
                                }
                            }
                            ForEach(model.dynamicRanges, id: \.self) { format in
                                FilterChip(label: GridModel.label(format), selected: draft.dynamicRanges.contains(format)) {
                                    draft.dynamicRanges.toggle(format)
                                }
                            }
                        }
                    }
                    if !model.narrators.isEmpty {
                        let narrators = shortened(model.narrators, whole: allNarrators) { draft.narrator == $0 }
                        group(String(localized: "Narrator"), count: model.narrators.count, whole: $allNarrators) {
                            ForEach(narrators) { narrator in
                                FilterChip(label: narrator.name, selected: draft.narrator?.id == narrator.id) {
                                    draft.narrator = draft.narrator?.id == narrator.id ? nil : narrator
                                }
                            }
                        }
                    }
                    if draft.filtered {
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
                    Button(String(localized: "Clear")) { draft = model.withoutFilters(draft) }
                        .disabled(!draft.filtered)
                        .accessibilityIdentifier("filters-clear")
                }
            }
            .containerBackground(palette.surface, for: .navigation)
        }
        .presentationDetents([.medium, .large], selection: $detent)
        .presentationDragIndicator(.visible)
        .presentationBackground(palette.surface)
        .onChange(of: draft) { send() }
        .onChange(of: model.paged.total) {
            if let total = model.paged.total { count = total }
        }
        .onDisappear {
            sending?.cancel()
            if model.query != draft { model.query = draft }
        }
    }

    /// The choices reach the grid once they've held still.
    private func send() {
        sending?.cancel()
        let draft = draft
        sending = Task {
            try? await Task.sleep(for: Self.settleTime)
            guard !Task.isCancelled else { return }
            model.query = draft
        }
    }

    /// All, Unwatched, In progress, Watched: a segmented control, or chips
    /// at the largest text sizes, where the segments would cut their words.
    @ViewBuilder private var watchStatus: some View {
        if typeSize.isAccessibilitySize {
            ForEach(model.playedFilters) { played in
                FilterChip(label: played.label, selected: draft.played == played) { draft.played = played }
            }
        } else {
            Picker(String(localized: "Watch status"), selection: $draft.played) {
                ForEach(model.playedFilters) { Text($0.label).tag($0) }
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
                if let whole, count > Self.shortLength + 3 {
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

    /// [values], or while not [whole] and long, the first few and any [chosen].
    private func shortened<T>(_ values: [T], whole: Bool, chosen: (T) -> Bool) -> [T] {
        guard !whole, values.count > Self.shortLength + 3 else { return values }
        return values.enumerated().filter { $0.offset < Self.shortLength || chosen($0.element) }.map(\.element)
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
                .frame(minHeight: 36)
                .background(selected ? palette.accent : palette.surfaceHover, in: RoundedRectangle(cornerRadius: 18))
                .contentShape(RoundedRectangle(cornerRadius: 18))
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

private extension Set {
    /// Takes [element] out if it's in, else puts it in.
    mutating func toggle(_ element: Element) {
        if remove(element) == nil { insert(element) }
    }
}
