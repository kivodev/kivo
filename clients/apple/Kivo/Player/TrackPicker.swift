import KivoCore
import SwiftUI

/// The detail page's audio or subtitle menu: Automatic first, named after
/// the track a play starts with, Off for subtitles, then each track by its
/// short name (`trackNames`) with what its title adds under it, and for
/// subtitles that can be searched for, Search subtitles last, which calls
/// [search]. It shows only with more than one track to choose, Off and
/// one, or a search.
struct TrackPicker: View {
    enum Kind { case audio, subtitles }

    let kind: Kind
    let tracks: [Track]
    let names: [String: TrackName]
    /// A track's ID, `off`, or nil for Automatic.
    @Binding var selection: String?
    /// The track Automatic plays, `off`, or nil for none.
    let automatic: String?
    /// Opens the subtitle search; nil where there's none.
    let search: (() -> Void)?

    init(kind: Kind, tracks: [Track], display: Display, selection: Binding<String?>, automatic: String?, search: (() -> Void)? = nil) {
        self.kind = kind
        self.tracks = tracks
        names = display.trackNames(tracks: tracks)
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
                        Text("Off").tag(String?.some(Subtitles.shared.OFF))
                    }
                    ForEach(tracks, id: \.id) { track in
                        Button {} label: {
                            Text(names[track.id]?.name ?? "")
                            if let note = names[track.id]?.note { Text(note) }
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
        if id == Subtitles.shared.OFF { return String(localized: "Off") }
        return id.flatMap { names[$0]?.name }
    }
}
