import KivoCore
import KivoUI
import SwiftUI

/// Settings: the user, who opens Account, and Link a device; a page per
/// topic, each holding the account's settings, kept on the server for every
/// app and browser the user signs in on, and this device's own; and About.
/// Each row pushes a page. There's no Save: each change applies at once and
/// says nothing unless it fails, when its page says why, with Retry.
struct SettingsView: View {
    @Environment(ServerModel.self) private var server
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        List {
            Section {
                NavigationLink {
                    AccountSettingsView { dismiss() }
                } label: {
                    profile
                }
                .accessibilityIdentifier("account")
                NavigationLink { LinkDeviceView() } label: {
                    Text(String(localized: "Link a device"))
                }
            }
            Section {
                NavigationLink { PlaybackSettingsView() } label: {
                    Text(String(localized: "Playback"))
                }
                NavigationLink { AudioSettingsView() } label: {
                    Text(String(localized: "Audio"))
                }
                NavigationLink { SubtitleSettingsView() } label: {
                    Text(String(localized: "Subtitles"))
                }
                NavigationLink { QualitySettingsView() } label: {
                    Text(String(localized: "Quality"))
                }
                NavigationLink { DownloadSettingsView() } label: {
                    Text(String(localized: "Downloads"))
                }
                NavigationLink { AppearanceSettingsView() } label: {
                    Text(String(localized: "Appearance"))
                }
                NavigationLink { LibrariesSettingsView() } label: {
                    Text(String(localized: "Home & Libraries"))
                }
            }
            Section {
                NavigationLink { AboutView() } label: {
                    Text(String(localized: "About"))
                }
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle(String(localized: "Settings"))
        .toolbar {
            ToolbarItem(placement: .confirmationAction) {
                Button(String(localized: "Done")) { dismiss() }
            }
        }
    }

    /// The user's avatar and name, with the server under them.
    private var profile: some View {
        HStack(spacing: 14) {
            Avatar(initial: server.user?.initial, size: 56)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 2) {
                Text(server.user?.username ?? "")
                    .font(.title3.weight(.semibold))
                Text(server.serverName)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 4)
    }
}

/// A change the server refused: why, and how to send it again.
struct SaveFailure {
    let message: String
    let retry: () -> Void
}

/// A failure in a settings group: what went wrong, in the danger colour, with Retry.
struct FailureRow: View {
    let message: String
    let retry: () -> Void
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        HStack {
            Text(message)
                .font(.footnote)
                .foregroundStyle(palette.danger)
            Spacer()
            Button(String(localized: "Retry"), action: retry)
                .buttonStyle(.borderless)
        }
    }
}
