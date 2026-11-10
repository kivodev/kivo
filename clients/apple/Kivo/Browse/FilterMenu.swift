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

    var body: some View {
        @Bindable var model = model
        Menu {
            Picker(selection: $model.query.played) {
                ForEach(model.playedFilters) { Text($0.label).tag($0) }
            } label: {
                Text("Show")
            }
            .pickerStyle(.inline)
            Section {
                if !model.genres.isEmpty {
                    several(String(localized: "Genre"), model.genres, in: \.genres) { $0 }
                }
                if !model.decades.isEmpty {
                    Picker(selection: $model.query.decade) {
                        Text("Any").tag(Int?.none)
                        ForEach(model.decades, id: \.self) { Text(GridModel.label(decade: $0)).tag(Int?.some($0)) }
                    } label: {
                        Text("Decade")
                    }
                    .pickerStyle(.menu)
                }
                if !model.contentRatings.isEmpty {
                    several(String(localized: "Content rating"), model.contentRatings, in: \.contentRatings) { $0 }
                }
                if !model.resolutions.isEmpty {
                    several(String(localized: "Resolution"), model.resolutions, in: \.resolutions, label: GridModel.label)
                }
                if !model.dynamicRanges.isEmpty {
                    several(String(localized: "Dynamic range"), model.dynamicRanges, in: \.dynamicRanges, label: GridModel.label)
                }
                if !model.narrators.isEmpty {
                    Picker(selection: $model.query.narrator) {
                        Text("Any").tag(GridModel.Narrator?.none)
                        ForEach(model.narrators) { Text($0.name).tag(GridModel.Narrator?.some($0)) }
                    } label: {
                        Text("Narrator")
                    }
                    .pickerStyle(.menu)
                }
            }
            if model.query.filtered {
                Section {
                    Button(String(localized: "Clear filters"), systemImage: "xmark.circle", action: model.clearFilters)
                    Button(String(localized: "Save as Smart List"), systemImage: "sparkles") { savingAsSmartList = true }
                }
            }
        } label: {
            FilterLabel(on: model.query.filtered)
        }
        .tint(palette.text)
        .accessibilityIdentifier("filter")
    }

    /// A submenu of [values] to take any of, named with those chosen.
    private func several(
        _ title: String,
        _ values: [String],
        in set: WritableKeyPath<GridModel.Query, Set<String>>,
        label: @escaping (String) -> String
    ) -> some View {
        let chosen = values.filter(model.query[keyPath: set].contains).map(label)
        return Menu {
            ForEach(values, id: \.self) { value in
                Toggle(label(value), isOn: member(value, of: set))
                    .menuActionDismissBehavior(.disabled)
            }
        } label: {
            Text(title)
            if !chosen.isEmpty {
                Text(chosen.joined(separator: ", "))
            }
        }
    }

    /// Whether [value] is in one of the query's sets, as a switch.
    private func member(_ value: String, of set: WritableKeyPath<GridModel.Query, Set<String>>) -> Binding<Bool> {
        Binding(
            get: { model.query[keyPath: set].contains(value) },
            set: { on in
                if on { model.query[keyPath: set].insert(value) } else { model.query[keyPath: set].remove(value) }
            }
        )
    }
}
