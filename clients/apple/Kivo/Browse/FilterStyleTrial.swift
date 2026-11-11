import SwiftUI

// FilterStyleTrial: everything in this file exists only to compare the
// filter sheet with the filter menu on a device. Once one is chosen, delete
// this file and the other style's file, put the chosen control where
// LibraryView uses FilterControl, and drop the setting from Appearance.

/// How a library's Filter button offers the filters, chosen in Settings ›
/// Appearance and kept on this device alone.
enum FilterStyle: String {
    case sheet = "Sheet"
    case menu = "Menu"

    /// Where the style is kept in UserDefaults.
    static let key = "trial.filterStyle"
}

/// The Filter button in the style chosen.
struct FilterControl: View {
    let model: GridModel
    @Binding var savingAsSmartList: Bool
    @AppStorage(FilterStyle.key) private var style = FilterStyle.sheet

    var body: some View {
        switch style {
        case .sheet: FilterSheetButton(model: model, savingAsSmartList: $savingAsSmartList)
        case .menu: FilterMenu(model: model, savingAsSmartList: $savingAsSmartList)
        }
    }
}

/// The filter style setting, for Appearance.
struct FilterStyleSection: View {
    @AppStorage(FilterStyle.key) private var style = FilterStyle.sheet

    var body: some View {
        Section {
            Picker(String(localized: "Filter style"), selection: $style) {
                Text("Sheet").tag(FilterStyle.sheet)
                Text("Menu").tag(FilterStyle.menu)
            }
        } footer: {
            Text("How a library's Filter button shows the filters.")
        }
    }
}
