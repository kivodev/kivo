import KivoCore
import KivoUI
import SwiftUI

/// Playback settings, the panel the player's gear opens: Audio & Subtitles
/// (opening the speech-bubble button's panel in its place), Quality (Auto, Original,
/// then the ladder below the file's bitrate), Version where there are
/// several, Speed, and Stream info.
struct PlaybackSettingsSheet: View {
    let model: any PlayerChoices
    @Environment(\.dismiss) private var dismiss

    static let speeds: [Float] = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2]

    var body: some View {
        NavigationStack {
            List {
                Section {
                    // Audio & Subtitles' own panel, in this one's place.
                    Button {
                        model.sheet = .tracks
                    } label: {
                        HStack {
                            Text("Audio & Subtitles").foregroundStyle(.primary)
                            Spacer()
                            Image(systemName: "chevron.right")
                                .font(.footnote.weight(.semibold))
                                .foregroundStyle(.tertiary)
                        }
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    if model.offline {
                        Text("Downloaded")
                    }
                    if !model.offline, !qualityOptions.isEmpty {
                        NavigationLink {
                            ChoiceList(title: String(localized: "Quality"), items: qualityOptions, id: \.self, label: qualityLabel, chosen: { $0.quality == model.quality }) {
                                model.chooseQuality($0)
                            }
                        } label: {
                            LabeledContent(String(localized: "Quality"), value: currentQuality)
                        }
                    }
                    if let work = model.work, work.versions.count > 1, !model.offline {
                        NavigationLink {
                            ChoiceList(title: String(localized: "Version"), items: work.versions, id: \.id, label: { "\(model.display.versionName(version: $0)) · \(Display.app.size(bytes: $0.sizeBytes))" }, chosen: { $0.id == model.version?.id }) {
                                model.chooseVersion($0.id)
                            }
                        } label: {
                            LabeledContent(String(localized: "Version"), value: model.version.map { model.display.versionName(version: $0) } ?? "")
                        }
                    }
                    NavigationLink {
                        ChoiceList(title: String(localized: "Speed"), items: Self.speeds, id: \.self, label: Self.speedLabel, chosen: { $0 == model.speed }) {
                            model.setSpeed($0)
                        }
                    } label: {
                        LabeledContent(String(localized: "Speed"), value: Self.speedLabel(model.speed))
                    }
                }
                if model.streamInfo != nil {
                    Section {
                        NavigationLink(String(localized: "Stream info")) { StreamInfoView(model: model) }
                    }
                }
            }
            .navigationTitle(String(localized: "Playback Settings"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button(String(localized: "Done")) { dismiss() }
                }
            }
        }
    }

    /// "1×", "1.25×"; normal speed says so.
    static func speedLabel(_ speed: Float) -> String {
        speed == 1 ? String(localized: "Normal") : "\(speed.formatted())×"
    }

    /// Auto, Original with the file's bitrate, and only the rungs below it.
    private var qualityOptions: [QualityOption] {
        let source = model.version?.bitrateBps?.int64Value
        return model.qualities.filter { option in
            guard let rung = option.rung, let source else { return true }
            return rung.bitrateBps < source
        }
    }

    /// A quality as the menu lists it, Original with the file's bitrate: "Original · 20 Mb/s".
    private func qualityLabel(_ option: QualityOption) -> String {
        model.display.quality(option: option, sourceBps: model.version?.bitrateBps)
    }

    /// The quality playing; one the server's ladder doesn't offer plays as Auto.
    private var currentQuality: String {
        guard let current = model.quality else { return "" }
        let option = model.qualities.first { $0.quality == current } ?? QualityOption(quality: Quality.Auto.shared, rung: nil)
        return qualityLabel(option)
    }
}

/// A pushed list of choices with a checkmark on the current one; choosing applies it and pops back.
private struct ChoiceList<Item, ID: Hashable>: View {
    let title: String
    let items: [Item]
    let id: KeyPath<Item, ID>
    let label: (Item) -> String
    let chosen: (Item) -> Bool
    let choose: (Item) -> Void
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        List {
            ForEach(items, id: id) { item in
                ChoiceRow(title: label(item), detail: nil, chosen: chosen(item)) {
                    choose(item)
                    dismiss()
                }
            }
        }
        .navigationTitle(title)
        .navigationBarTitleDisplayMode(.inline)
    }
}
