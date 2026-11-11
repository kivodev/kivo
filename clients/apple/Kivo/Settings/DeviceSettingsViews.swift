import KivoCore
import KivoUI
import SwiftUI

/// Downloads: the quality videos download at, and whether they may use
/// cellular data.
struct DownloadSettingsView: View {
    @Environment(AppModel.self) private var app

    var body: some View {
        SettingsForm(title: String(localized: "Downloads"), device: { settings, save in
            Section {
                Picker(String(localized: "Quality"), selection: save.binding(settings.downloadQuality) { quality in
                    try await app.core.setDownloadQuality(quality: quality)
                }) {
                    ForEach(DownloadQuality.allCases, id: \.self) { quality in
                        Text(app.display.downloadQuality(quality: quality)).tag(quality)
                    }
                }
            } header: {
                DeviceHeading()
            }
            Section {
                Toggle(String(localized: "Allow cellular downloads"), isOn: save.binding(settings.cellularDownloads) { allowed in
                    try await app.core.setCellularDownloads(allowed: allowed)
                })
            }
        })
    }
}

/// Appearance: the theme.
struct AppearanceSettingsView: View {
    @Environment(AppModel.self) private var app

    var body: some View {
        @Bindable var app = app
        SettingsForm(title: String(localized: "Appearance"), device: { _, _ in
            Section {
                Picker(String(localized: "Theme"), selection: $app.theme) {
                    Text("System").tag(Theme.system)
                    Text("Dark").tag(Theme.dark)
                    Text("Light").tag(Theme.light)
                }
            } header: {
                DeviceHeading()
            }
        })
    }
}
