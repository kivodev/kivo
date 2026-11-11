import KivoCore
import KivoUI
import SwiftUI

/// Audio & Subtitles, the panel the player's speech-bubble button and
/// Playback settings' row open: Audio with the Dialogue boost switch above
/// its tracks, and Subtitles with Off, each track by its short name
/// (`pickerName`) and the ones playing checked, and where the play's
/// subtitles can be searched for, Search subtitles last, which shows even
/// without a subtitle track and pushes the search. Where the panel is wide
/// they're two lists side by side that each scroll on their own, else two
/// sections of one list. Each list opens scrolled to its checked row. A
/// choice applies at once, starting a new play at the position where it
/// must, and the panel stays open until Done.
struct TracksPanel: View {
    let model: any PlayerChoices
    @State private var sideBySide = false

    /// The panel's width as a popover, for two lists of short names.
    static let width: CGFloat = 620

    /// The narrowest panel that holds two lists.
    private static let sideBySideWidth: CGFloat = 520

    var body: some View {
        Group {
            if let version = model.version {
                let audio = TrackSection(title: String(localized: "Audio"), rows: audioRows(version), boost: model.dialogueBoost, search: false)
                let subtitles = TrackSection(title: String(localized: "Subtitles"), rows: subtitleRows(version), boost: nil, search: model.searchesSubtitles)
                let sections = [audio, subtitles].filter { !$0.rows.isEmpty || $0.boost != nil }
                if sideBySide {
                    // A gap between the lists, which would otherwise meet edge to edge.
                    HStack(spacing: KivoTokens.spacing * 4) {
                        ForEach(sections) { section in
                            TrackList(model: model, sections: [section])
                        }
                    }
                } else {
                    TrackList(model: model, sections: sections)
                }
            }
        }
        .onGeometryChange(for: Bool.self) { $0.size.width >= Self.sideBySideWidth } action: { sideBySide = $0 }
        .navigationTitle(String(localized: "Audio & Subtitles"))
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .confirmationAction) {
                Button(String(localized: "Done")) { model.sheet = nil }
            }
        }
    }

    /// The audio tracks to choose, with more than one; a download has the one it kept.
    private func audioRows(_ version: Version) -> [TrackRow] {
        let audio = version.tracks.filter { $0.type == .audio && model.offers($0) }
        guard audio.count > 1 else { return [] }
        return audio.map { track in
            TrackRow(id: track.id, name: track.pickerName, chosen: track.id == model.audioTrackId) { model.chooseAudio(track.id) }
        }
    }

    /// Off and the subtitle tracks; a download has the ones it kept. Off alone where there are none to choose but a search.
    private func subtitleRows(_ version: Version) -> [TrackRow] {
        let subtitles = version.tracks.filter { $0.type == .subtitle && model.offers($0) }
        guard !subtitles.isEmpty || model.searchesSubtitles else { return [] }
        let off = SUBTITLES_OFF
        let offRow = TrackRow(id: off, name: TrackName(name: String(localized: "Off"), note: nil), chosen: model.subtitle == off) {
            model.chooseSubtitle(off)
        }
        return [offRow] + subtitles.map { track in
            TrackRow(id: track.id, name: track.pickerName, chosen: model.subtitle == track.id) { model.chooseSubtitle(track.id) }
        }
    }
}

/// A track or Off: its short name, whether it plays, and what choosing it does.
private struct TrackRow: Identifiable {
    let id: String
    let name: TrackName?
    let chosen: Bool
    let choose: () -> Void
}

/// A kind's rows under its heading, Audio with the Dialogue boost switch above them, Subtitles with Search subtitles below.
private struct TrackSection: Identifiable {
    let title: String
    let rows: [TrackRow]
    let boost: DialogueBoost?
    let search: Bool

    var id: String { title }
}

/// Sections in one list, opened scrolled to the first section's checked row.
private struct TrackList: View {
    let model: any PlayerChoices
    let sections: [TrackSection]

    var body: some View {
        ScrollViewReader { proxy in
            List {
                ForEach(sections) { section in
                    Section(section.title) {
                        if let boost = section.boost {
                            Toggle(String(localized: "Dialogue boost"), isOn: Binding(get: { boost.enabled }, set: { model.boostDialogue($0) }))
                                .accessibilityIdentifier("dialogue-boost")
                        }
                        ForEach(section.rows) { row in
                            ChoiceRow(title: row.name?.name ?? "", detail: row.name?.note, chosen: row.chosen, choose: row.choose)
                                .id(row.id)
                        }
                        if section.search {
                            NavigationLink(String(localized: "Search subtitles…")) {
                                SubtitleSearchView(
                                    display: model.display,
                                    search: { try await model.searchSubtitles() },
                                    download: { try await model.downloadSubtitle($0) }
                                )
                            }
                            .accessibilityIdentifier("search-subtitles")
                        }
                    }
                }
            }
            .task {
                if let chosen = sections.first?.rows.first(where: \.chosen) {
                    proxy.scrollTo(chosen.id, anchor: .center)
                }
            }
        }
    }
}
