import KivoCore
import KivoUI
import SwiftUI

// FilterStyleTrial: the filter menu, one of the two styles being compared.

/// The Filter button as a pull-down menu: watch status, then a submenu per
/// group the library offers, each naming what's chosen in it, then Clear
/// filters and Save as Smart List while any filter is on. Each choice
/// applies at once; a group taking several stays open across taps.
struct FilterMenu: View {
    let model: GridModel
    @Binding var savingAsSmartList: Bool
    @Environment(\.kivoPalette) private var palette

    /// All, then each watch status.
    private static let statuses: [WorkPlayState?] = [nil, .unplayed, .inProgress, .played]

    private var chosen: GridFilters { model.state.filters }

    var body: some View {
        Menu {
            Picker(selection: one(of: .watchStatus)) {
                ForEach(Self.statuses, id: \.self) { status in
                    Text(model.label(played: status)).tag(status.map { GridFilter.Played(state: $0).key })
                }
            } label: {
                Text("Show")
            }
            .pickerStyle(.inline)
            Section {
                let quality = values(of: .quality)
                several(String(localized: "Genre"), values(of: .genre))
                one(String(localized: "Decade"), .decade)
                several(String(localized: "Content rating"), values(of: .contentRating))
                several(String(localized: "Resolution"), quality.filter { $0 is GridFilter.Resolution })
                several(String(localized: "Dynamic range"), quality.filter { $0 is GridFilter.DynamicRange })
                one(String(localized: "Narrator"), .narrator)
            }
            if !chosen.isEmpty {
                Section {
                    Button(String(localized: "Clear filters"), systemImage: "xmark.circle") { model.clearFilters() }
                    Button(String(localized: "Save as Smart List"), systemImage: "sparkles") { savingAsSmartList = true }
                }
            }
        } label: {
            FilterLabel(on: !chosen.isEmpty)
        }
        .tint(palette.text)
        .accessibilityIdentifier("filter")
    }

    /// The values [group] offers; none when the library has none of it.
    private func values(of group: FilterGroupKind) -> [GridFilter] {
        model.state.offer.first { $0.kind == group }?.values ?? []
    }

    /// The one value on in [group], by its key; nil turns the group off.
    private func one(of group: FilterGroupKind) -> Binding<String?> {
        Binding(
            get: { chosen.chosen.first { $0.group == group }?.key },
            set: { key in
                let picked = values(of: group).first { $0.key == key }
                model.choose(picked.map { chosen.choosing(filter: $0) } ?? chosen.clearing(group: group))
            }
        )
    }

    /// A submenu of [group]'s values to take one of, or Any, named with what's chosen; nothing when it offers none.
    @ViewBuilder private func one(_ title: String, _ group: FilterGroupKind) -> some View {
        let values = values(of: group)
        if !values.isEmpty {
            Menu {
                Picker(title, selection: one(of: group)) {
                    Text("Any").tag(String?.none)
                    ForEach(values, id: \.key) { Text(model.label($0)).tag(String?.some($0.key)) }
                }
                .pickerStyle(.inline)
            } label: {
                Text(title)
                if let on = chosen.chosen.first(where: { $0.group == group }) { Text(model.label(on)) }
            }
        }
    }

    /// A submenu of [values] to take any of, named with those chosen; nothing when there are none.
    @ViewBuilder private func several(_ title: String, _ values: [GridFilter]) -> some View {
        if !values.isEmpty {
            let on = values.filter { chosen.has(filter: $0) }.map { model.label($0) }
            Menu {
                ForEach(values, id: \.key) { value in
                    Toggle(model.label(value), isOn: Binding(get: { chosen.has(filter: value) }, set: { _ in model.toggle(value) }))
                        .menuActionDismissBehavior(.disabled)
                }
            } label: {
                Text(title)
                if !on.isEmpty {
                    Text(on.joined(separator: ", "))
                }
            }
        }
    }
}
