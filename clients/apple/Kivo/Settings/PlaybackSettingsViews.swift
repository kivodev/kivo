import KivoCore
import KivoUI
import SwiftUI

/// Playback: whether up next plays the next episode by itself, and whether
/// a show remembers the tracks last chosen, kept on the server, so they
/// follow the user to every device.
struct PlaybackSettingsView: View {
    @Environment(ServerModel.self) private var server

    var body: some View {
        SettingsForm(title: String(localized: "Playback"), account: { playback, save in
            Section {
                Toggle(String(localized: "Play next episode automatically"), isOn: save.binding(playback.autoplayNext) { autoplay in
                    try await server.session.account.setAutoplayNext(autoplay: autoplay)
                })
            } header: {
                AccountHeading()
            }
            Section {
                Toggle(String(localized: "Remember choices per show"), isOn: save.binding(playback.rememberTracks) { remember in
                    try await server.session.account.setRememberTracks(remember: remember)
                })
            } footer: {
                Text("Episodes use the tracks last chosen for the show.")
            }
        })
    }
}

/// Audio: the audio language, kept on the server; then the most channels
/// this device is sent, and whether its plays boost dialogue, as the
/// player's switch does.
struct AudioSettingsView: View {
    @Environment(ServerModel.self) private var server
    @Environment(AppModel.self) private var app

    /// Maximum audio channels' choices, each with the channels it keeps;
    /// Auto keeps 0, which the core stores as none.
    static let channelChoices: [(label: String, channels: Int32)] = [
        (String(localized: "Auto"), 0),
        (String(localized: "Stereo"), 2),
        ("5.1", 6),
        ("7.1", 8),
    ]

    var body: some View {
        SettingsForm(title: String(localized: "Audio")) { playback, save in
            Section {
                Picker(String(localized: "Audio language"), selection: save.binding(playback.audioLanguage) { language in
                    try await server.session.account.setAudioLanguage(language: language)
                }) {
                    Text("Original language").tag(String?.none)
                    languageOptions(selected: playback.audioLanguage)
                }
            } header: {
                AccountHeading()
            }
        } device: { settings, save in
            Section {
                Picker(String(localized: "Maximum audio channels"), selection: save.binding(settings.audio.maxChannels?.int32Value ?? 0) { channels in
                    try await app.core.setMaxAudioChannels(channels: channels)
                }) {
                    ForEach(Self.channelChoices, id: \.channels) { choice in
                        Text(choice.label).tag(choice.channels)
                    }
                }
            } header: {
                DeviceHeading()
            } footer: {
                Text("Audio with more channels is mixed down on the server.")
            }
            Section {
                Toggle(String(localized: "Dialogue boost"), isOn: save.binding(settings.dialogueBoost) { on in
                    try await app.core.setDialogueBoost(on: on)
                })
            } footer: {
                Text("Makes speech clearer and evens out loud scenes, playing some audio as AAC.")
            }
        }
    }
}

/// Subtitles: when subtitles show and in which language, kept on the
/// server; then the size and background of plain text subtitles on this
/// device.
struct SubtitleSettingsView: View {
    @Environment(ServerModel.self) private var server
    @Environment(AppModel.self) private var app

    var body: some View {
        @Bindable var app = app
        SettingsForm(title: String(localized: "Subtitles")) { playback, save in
            Section {
                Picker(String(localized: "Show subtitles"), selection: save.binding(playback.subtitleMode) { mode in
                    try await server.session.account.setSubtitleMode(mode: mode)
                }) {
                    Text("Off").tag("none")
                    Text("Only forced").tag("forced")
                    Text("Foreign audio only").tag("smart")
                    Text("Always").tag("always")
                }
                Picker(String(localized: "Subtitle language"), selection: save.binding(playback.subtitleLanguage) { language in
                    try await server.session.account.setSubtitleLanguage(language: language)
                }) {
                    Text("Server default").tag(String?.none)
                    languageOptions(selected: playback.subtitleLanguage)
                }
                .disabled(playback.subtitleMode == "none")
            } header: {
                AccountHeading()
            }
        } device: { _, _ in
            Section {
                Picker(String(localized: "Size"), selection: $app.subtitleSize) {
                    Text("Small").tag(SubtitleSize.small)
                    Text("Medium").tag(SubtitleSize.medium)
                    Text("Large").tag(SubtitleSize.large)
                }
                Picker(String(localized: "Background"), selection: $app.subtitleBackground) {
                    Text("None").tag(SubtitleBackground.none)
                    Text("Shadow").tag(SubtitleBackground.shadow)
                    Text("Box").tag(SubtitleBackground.box)
                }
            } header: {
                DeviceHeading()
            } footer: {
                Text("Styled subtitles keep their own look.")
            }
        }
    }
}

/// Quality: the most a play uses on the server's local network and away
/// from it, from Auto, Original and the server's ladder, kept on this
/// device. A stored rung the ladder doesn't have shows as Auto.
struct QualitySettingsView: View {
    @Environment(ServerModel.self) private var server
    @State private var qualities: [QualityOption] = []
    /// The most the server lets this user's remote streams use, if it limits them.
    @State private var cap: Int64?
    @State private var failure: String?

    var body: some View {
        SettingsForm(title: String(localized: "Quality"), device: { settings, save in
            if !qualities.isEmpty {
                Section {
                    Picker(String(localized: "Local network quality"), selection: save.binding(shown(settings.localQuality)) { quality in
                        try await server.app.core.setLocalQuality(quality: quality)
                    }) {
                        options
                    }
                    Picker(String(localized: "Remote quality"), selection: save.binding(shown(settings.remoteQuality)) { quality in
                        try await server.app.core.setRemoteQuality(quality: quality)
                    }) {
                        options
                    }
                } header: {
                    DeviceHeading()
                } footer: {
                    if let cap {
                        Text("Limited to \(Display.app.bitrate(bps: cap)) by the server.")
                    }
                }
            } else if let failure {
                Section {
                    FailureRow(message: failure) { Task { await load() } }
                } header: {
                    DeviceHeading()
                }
            } else {
                Section {
                    ProgressView().frame(maxWidth: .infinity)
                } header: {
                    DeviceHeading()
                }
            }
        })
        .task { await load() }
    }

    private var options: some View {
        ForEach(qualities, id: \.self) { option in
            Text(server.display.quality(option: option, sourceBps: nil)).tag(option.quality)
        }
    }

    /// The stored quality, or Auto when the server's ladder doesn't have it.
    private func shown(_ quality: Quality) -> Quality {
        quality.onLadder(rungs: qualities.compactMap(\.rung))
    }

    private func load() async {
        do {
            async let menu = server.session.playback.qualityMenu()
            async let user = server.session.account.currentUser()
            (qualities, cap) = try await (menu, user.limits.remoteQualityCapBps?.int64Value)
            failure = nil
        } catch {
            failure = error.problemSentence
        }
    }
}

/// The audio and subtitle languages offered: the system's two-letter
/// languages (base languages, as playback compares tracks), named in this
/// locale and sorted by name, with [selected] among them when it isn't one of them.
private func languageOptions(selected: String?) -> some View {
    let languages = Locale.LanguageCode.isoLanguageCodes.map(\.identifier).filter { $0.count == 2 }
    let codes = selected.map { languages.contains($0) ? languages : [$0] + languages } ?? languages
    var names = Set<String>()
    let named = codes
        .map { (code: $0, name: Locale.current.localizedString(forLanguageCode: $0) ?? $0) }
        // Languages the system has a name for, once each, and the one selected.
        .filter { $0.code == selected || $0.name != $0.code && names.insert($0.name).inserted }
        .sorted { $0.name.localizedCompare($1.name) == .orderedAscending }
    return ForEach(named, id: \.code) { language in
        Text(language.name).tag(String?.some(language.code))
    }
}
