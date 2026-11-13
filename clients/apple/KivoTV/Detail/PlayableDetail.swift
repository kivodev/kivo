import KivoCore
import KivoUI
import SwiftUI

/// A movie, an episode or another video: its title, details and overview;
/// Play or Resume, with Play from beginning beside Resume, My List,
/// watched, the version, audio and subtitle choices, which carry into
/// playback, and ⋯; under them the time left and when it ends once it's
/// started. An episode has its series above its title, which opens the
/// series, and its season's episodes below, opening on it. Then more like
/// this, trailers, cast, extras and collections.
struct PlayableDetail: View {
    let work: WorkSummary
    let page: WorkPageState
    let model: WorkPage
    @Environment(ServerModel.self) private var server
    @Environment(\.openWork) private var openWork
    @Environment(\.kivoPalette) private var palette
    /// Subtitles downloaded here, by version, shown until the work is read again with them.
    @State private var downloaded: [(versionId: String, track: Track)] = []
    @State private var searching = false

    /// The version selected: the one chosen, else the one the user last
    /// played while the work still has it, else the first.
    private var version: Version? {
        guard let read = page.work else { return nil }
        let shown = downloaded.reduce(read) { $0.withTrack(versionId: $1.versionId, track: $1.track) }
        return shown.selectedVersion(versionId: page.choice.versionId)
    }

    var body: some View {
        DetailPage(art: work.images.still ?? work.images.backdrop ?? work.images.poster) {
            VStack(alignment: .leading, spacing: KivoTokens.spacing * 6) {
                if let episode = work.episode {
                    Button(episode.seriesTitle) { openWork(WorkRoute(seriesOf: episode)) }
                        .buttonStyle(.borderless)
                        .font(.headline)
                }
                HeroTitle(work: work, details: DetailWords.details(work, genres: page.work?.genres), overview: page.work.map { $0.overview } ?? " ")
                actions
                TimelineView(.everyMinute) { context in
                    Text(DetailWords.timeLeft(work, from: context.date) ?? " ")
                        .kivoStyle(KivoTokens.TextSize.xs)
                        .foregroundStyle(palette.textMuted)
                }
            }
        } rows: {
            if let episode = work.episode {
                SeasonRow(title: String(localized: "Season \(episode.seasonNumber)"), episodes: page.episodes?.getOrNull() as? [WorkSummary], focus: work.id)
            }
            if let read = page.work {
                DetailRows(work: read)
            }
        }
        .sheet(isPresented: $searching) {
            if let version {
                SubtitleSearchList(
                    display: server.display,
                    search: { try await server.session.playback.subtitleResults(version: version) },
                    download: { result in
                        let track = try await server.session.playback.addSubtitle(version: version, resultId: result.id)
                        downloaded.append((version.id, track))
                        model.chooseSubtitle(id: track.id)
                    }
                )
            }
        }
    }

    @ViewBuilder private var actions: some View {
        if !work.available {
            VStack(alignment: .leading, spacing: KivoTokens.spacing * 4) {
                Text("This file is missing")
                    .kivoStyle(KivoTokens.TextSize.base, weight: KivoTokens.Weight.semibold)
                    .foregroundStyle(palette.danger)
                HStack(spacing: KivoTokens.spacing * 6) {
                    UserStateButtons(work: work, page: page)
                    MoreMenu(work: work)
                }
            }
        } else {
            HStack(spacing: KivoTokens.spacing * 6) {
                let resume = DetailWords.resume(work, read: page.work, versionId: page.choice.versionId)
                Button {
                    play(fromStart: false)
                } label: {
                    Label(resume ?? String(localized: "Play"), systemImage: Symbols.play)
                }
                if resume != nil {
                    Button {
                        play(fromStart: true)
                    } label: {
                        Image(systemName: Symbols.playFromBeginning)
                            .accessibilityLabel(Text("Play from beginning"))
                    }
                    .buttonBorderShape(.circle)
                }
                UserStateButtons(work: work, page: page)
                if let read = page.work {
                    TrackChoices(work: read, version: version, choice: page.choice, model: model) { searching = true }
                }
                MoreMenu(work: work)
            }
        }
    }

    /// Plays from the server with the version and tracks chosen here.
    private func play(fromStart: Bool) {
        let choice = PlayChoice(workId: work.id, versionId: version?.id, audioTrackId: page.choice.audioTrackId, subtitleTrackId: page.choice.subtitleTrackId)
        server.player.play(choice, fromStart: fromStart)
    }
}

/// The version, audio and subtitle choices, each a menu of its own among a
/// hero's buttons, naming what plays: the version only with more than one,
/// and each TrackPicker Automatic until a track is chosen. Another version
/// starts its tracks at Automatic. Subtitles' Search subtitles calls [search].
private struct TrackChoices: View {
    let work: WorkDetail
    let version: Version?
    let choice: PlayChoice
    let model: WorkPage
    let search: () -> Void

    var body: some View {
        let audio = TrackPicker(
            kind: .audio,
            tracks: version?.tracks.filter { $0.type == .audio } ?? [],
            selection: Binding(get: { choice.audioTrackId }, set: { model.chooseAudio(id: $0) }),
            automatic: version?.audioTrackId
        )
        let subtitles = TrackPicker(
            kind: .subtitles,
            tracks: version?.tracks.filter { $0.type == .subtitle } ?? [],
            selection: Binding(get: { choice.subtitleTrackId }, set: { model.chooseSubtitle(id: $0) }),
            automatic: version?.subtitleTrackId ?? SUBTITLES_OFF,
            search: SubtitleSearch.shared.offered(version: version) ? search : nil
        )
        if work.versions.count > 1 {
            Menu {
                Picker(String(localized: "Version"), selection: Binding(get: { version?.id ?? "" }, set: { model.chooseVersion(id: $0) })) {
                    ForEach(work.versions, id: \.id) { version in
                        Text("\(version.name) · \(Display.app.size(bytes: version.sizeBytes))").tag(version.id)
                    }
                }
                .pickerStyle(.inline)
            } label: {
                Label(version?.name ?? "", systemImage: Symbols.version)
            }
        }
        if TrackPicker.shows(.audio, tracks: audio.tracks) {
            Menu { audio } label: { Label(audio.value, systemImage: Symbols.audio) }
        }
        if TrackPicker.shows(.subtitles, tracks: subtitles.tracks, searchable: subtitles.search != nil) {
            Menu { subtitles } label: { Label(subtitles.value, systemImage: Symbols.subtitles) }
        }
    }
}
