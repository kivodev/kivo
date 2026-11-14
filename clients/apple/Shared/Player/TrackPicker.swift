import KivoCore
import SwiftUI

/// The detail page's audio or subtitle menu: Automatic first, named after
/// the track a play starts with, Off for subtitles, then each track by its
/// short name (`pickerName`) with its title under it, and for
/// subtitles that can be searched for, Search subtitles last, which calls
/// [search]. It shows only with more than one track to choose, Off and
/// one, or a search.
struct TrackPicker: View {
    enum Kind { case audio, subtitles }

    let kind: Kind
    let tracks: [Track]
    /// A track's ID, `off`, or nil for Automatic.
    @Binding var selection: String?
    /// The track Automatic plays, `off`, or nil for none.
    let automatic: String?
    /// Opens the subtitle search; nil where there's none.
    let search: (() -> Void)?

    init(kind: Kind, tracks: [Track], selection: Binding<String?>, automatic: String?, search: (() -> Void)? = nil) {
        self.kind = kind
        self.tracks = tracks
        _selection = selection
        self.automatic = automatic
        self.search = search
    }

    /// Whether there's a choice to make: more than one audio track, or a subtitle track besides Off or a search.
    static func shows(_ kind: Kind, tracks: [Track], searchable: Bool = false) -> Bool {
        kind == .audio ? tracks.count > 1 : !tracks.isEmpty || searchable
    }

    var title: String {
        kind == .audio ? String(localized: "Audio") : String(localized: "Subtitles")
    }

    /// What plays: the track chosen, else the one Automatic plays.
    var value: String {
        name(selection ?? automatic) ?? String(localized: "Automatic")
    }

    var body: some View {
        if Self.shows(kind, tracks: tracks, searchable: search != nil) {
            Section {
                Picker(title, selection: $selection) {
                    Text(automaticLabel).tag(String?.none)
                    if kind == .subtitles {
                        Text("Off").tag(String?.some(SUBTITLES_OFF))
                    }
                    ForEach(tracks, id: \.id) { track in
                        Button {} label: {
                            Text(track.pickerName.name)
                            if let note = track.pickerName.note { Text(note).lineLimit(2) }
                        }
                        .tag(String?.some(track.id))
                    }
                }
                .pickerStyle(.inline)
                if let search {
                    Button(String(localized: "Search subtitles…"), systemImage: "magnifyingglass", action: search)
                }
            }
        }
    }

    /// "Automatic (English · 5.1)".
    private var automaticLabel: String {
        name(automatic).map { String(localized: "Automatic (\($0))") } ?? String(localized: "Automatic")
    }

    private func name(_ id: String?) -> String? {
        if id == SUBTITLES_OFF { return String(localized: "Off") }
        return id.flatMap { id in tracks.first { $0.id == id }?.name }
    }
}
