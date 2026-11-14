import KivoCore
import KivoUI
import SwiftUI

/// A movie, an episode or another video: Play or Resume, My List and Mark
/// as Watched, then the overview, the version, audio and subtitle choices,
/// which carry into playback, trailers, cast, extras and collections. An episode has
/// its series above its title and its season's episodes below, a shelf of
/// cards in a regular width and a list in a compact one. Once it's in
/// progress, a round Play from Beginning sits beside Resume, with the time
/// left and when it ends under them. Download sits beside My List and Watched. A
/// ready download is what Play and Resume play, with "Downloaded"
/// under the actions and Stream Instead and Remove Download in the ⋯ menu.
struct PlayableDetail: View {
    /// What shows of the work: its page's summary once read.
    let work: WorkSummary
    let page: WorkPageState
    /// Where the version and track choices go.
    let model: WorkPage
    @Environment(ServerModel.self) private var server
    @Environment(\.openWork) private var openWork
    @Environment(\.kivoPalette) private var palette
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    /// Subtitles downloaded here, by version, shown until the work is read again with them.
    @State private var downloaded: [(versionId: String, track: Track)] = []

    /// The version chosen; nil selects the work's default one.
    private var versionId: String? { page.choice.versionId }

    /// The version selected: the one chosen, else the one the user last
    /// played while the work still has it, else the first.
    private var version: Version? {
        guard let read = page.work else { return nil }
        let shown = downloaded.reduce(read) { $0.withTrack(versionId: $1.versionId, track: $1.track) }
        return shown.selectedVersion(versionId: versionId)
    }

    /// The season's episodes; nil until they're read.
    private var season: [WorkSummary]? {
        page.episodes?.getOrNull() as? [WorkSummary]
    }

    /// Why the season's episodes couldn't be read, when the server refused them.
    private var seasonProblem: RequestProblem? {
        page.episodes?.problemOrNull().flatMap { $0.isUnreachable ? nil : $0 }
    }

    var body: some View {
        DetailPage(
            work: work,
            art: work.images.still ?? work.images.backdrop ?? work.images.poster,
            eyebrow: work.episode.map { episode in
                (label: episode.seriesTitle, open: { openWork(WorkRoute(seriesOf: episode)) })
            },
            details: details
        ) {
            actions
                .actionRowInsets()
            Overview(text: page.work?.overview)
            if let read = page.work {
                TrackChoices(work: read, version: version, choice: page.choice, model: model) { versionId, track in
                    downloaded.append((versionId, track))
                }
            }
            if let episode = work.episode, let seasonId = episode.seasonId, season?.isEmpty != true {
                SectionHeading(title: String(localized: "Season \(episode.seasonNumber)"))
                    .detailRow(EdgeInsets(top: 20, leading: 20, bottom: 4, trailing: 20))
                if let season, horizontalSizeClass == .regular {
                    EpisodeShelf(episodes: season, current: work.id, focus: work.id)
                } else if let season {
                    ForEach(season, id: \.id) { episode in
                        EpisodeRow(episode: episode, current: episode.id == work.id)
                    }
                } else if let seasonProblem {
                    InlineFailure(title: String(localized: "Couldn't load this season."), problem: seasonProblem, retry: model.retry)
                } else if horizontalSizeClass == .regular {
                    EpisodeShelfSkeleton()
                } else {
                    EpisodeRowSkeletons(count: server.session.library.known(id: seasonId)?.season?.episodeCount?.intValue)
                }
            }
            if let read = page.work {
                CardRow(title: String(localized: "More like this"), works: read.similar ?? [])
                TrailersRow(work: read)
                CastRow(credits: read.credits)
                CardRow(title: String(localized: "Extras"), works: read.otherExtras())
                ForEach(read.collections, id: \.id) { collection in
                    CollectionRow(collection: collection)
                }
            }
            Color.clear.frame(height: KivoTokens.spacing * 6).detailRow()
        } actions: {
            menu
        }
    }

    private var details: [String] {
        DetailWords.details(work, genres: page.work?.genres)
    }

    /// "Resume 5:56", on the version selected.
    private var resumeLabel: String? {
        DetailWords.resume(work, read: page.work, versionId: versionId)
    }

    /// The download Play plays, once it's ready.
    private var download: DownloadItem? {
        server.readyDownload(of: work.id)
    }

    @ViewBuilder private var actions: some View {
        if !work.available {
            VStack(spacing: KivoTokens.spacing * 3) {
                Text("This file is missing")
                    .font(.headline)
                    .foregroundStyle(palette.danger)
                HStack(spacing: KivoTokens.spacing * 3) {
                    UserStateButtons(work: work, page: page)
                }
                .controlSize(.large)
            }
            .frame(maxWidth: .infinity)
        } else {
            VStack(spacing: KivoTokens.spacing * 3) {
                // The end time moves on with the clock.
                TimelineView(.everyMinute) { context in
                    ActionRow(work: work, page: page, secondary: DetailWords.timeLeft(work, from: context.date)) {
                        PlayButton(label: resumeLabel ?? String(localized: "Play")) {
                            play(fromStart: false)
                        }
                        if resumeLabel != nil {
                            RestartButton { play(fromStart: true) }
                        }
                    } extra: {
                        downloadButton
                    }
                }
                DownloadBadge(workId: work.id)
            }
        }
    }

    private var downloadButton: some View {
        DownloadButton(work: work, choice: choice)
    }

    @ViewBuilder private var menu: some View {
        if let item = server.download(of: work.id) {
            Section {
                if work.available, download != nil {
                    Button {
                        stream(fromStart: false)
                    } label: {
                        Label(String(localized: "Stream instead"), systemImage: "antenna.radiowaves.left.and.right")
                    }
                }
                Button(role: .destructive) {
                    // A download that can't be removed stays in the list, and the app says why.
                    Task { server.tell(try? await server.session.downloads?.delete(id: item.id)) }
                } label: {
                    Label(String(localized: "Remove download"), systemImage: "trash")
                }
            }
        }
        if let episode = work.episode {
            Section {
                if let previous = episode.previousId {
                    Button {
                        openWork(WorkRoute(id: previous, kind: .episode))
                    } label: {
                        Label(String(localized: "Previous episode"), systemImage: "backward.end")
                    }
                }
                if let next = episode.nextId {
                    Button {
                        openWork(WorkRoute(id: next, kind: .episode))
                    } label: {
                        Label(String(localized: "Next episode"), systemImage: "forward.end")
                    }
                }
                Button {
                    openWork(WorkRoute(seriesOf: episode))
                } label: {
                    Label(String(localized: "Go to series"), systemImage: "tv")
                }
            }
        }
        Section {
            UserStateMenuItems(work: work)
            GoToLibraryItem(libraryId: work.libraryId)
        }
    }

    /// Plays the download when there's one ready, with the subtitle chosen where it has it, else streams.
    private func play(fromStart: Bool) {
        if let download {
            server.player.play(PlayChoice(workId: work.id, subtitleTrackId: page.choice.subtitleTrackId), fromStart: fromStart, downloadId: download.id)
        } else {
            stream(fromStart: fromStart)
        }
    }

    /// The work with the version and tracks chosen here.
    private var choice: PlayChoice {
        PlayChoice(workId: work.id, versionId: version?.id, audioTrackId: page.choice.audioTrackId, subtitleTrackId: page.choice.subtitleTrackId)
    }

    /// Plays from the server, at this device's streaming quality.
    private func stream(fromStart: Bool) {
        server.player.play(choice, fromStart: fromStart)
    }
}

/// The version, audio and subtitle choices, as small
/// neutral menus: the version only with more than one, and the
/// TrackPickers, each Automatic until a track is chosen; another version
/// starts its tracks at Automatic. Subtitles' Search subtitles opens the
/// search in a sheet; a subtitle downloaded there is the one chosen for the
/// next play, and [downloaded] shows it among the tracks.
private struct TrackChoices: View {
    let work: WorkDetail
    let version: Version?
    let choice: PlayChoice
    let model: WorkPage
    let downloaded: (_ versionId: String, _ track: Track) -> Void
    @Environment(ServerModel.self) private var server
    @Environment(\.kivoPalette) private var palette
    @State private var searching = false

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
            search: SubtitleSearch.shared.offered(version: version) ? { searching = true } : nil
        )
        let showsAudio = TrackPicker.shows(.audio, tracks: audio.tracks)
        let showsSubtitles = TrackPicker.shows(.subtitles, tracks: subtitles.tracks, searchable: subtitles.search != nil)
        if work.available, work.versions.count > 1 || showsAudio || showsSubtitles {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: KivoTokens.spacing * 2) {
                    if work.versions.count > 1 {
                        choice(String(localized: "Version"), systemImage: Symbols.version, value: versionName) {
                            Picker(String(localized: "Version"), selection: Binding(get: { version?.id ?? "" }, set: { model.chooseVersion(id: $0) })) {
                                ForEach(work.versions, id: \.id) { version in
                                    Text("\(version.name) · \(Display.app.size(bytes: version.sizeBytes))").tag(version.id)
                                }
                            }
                        }
                    }
                    if showsAudio {
                        choice(audio.title, systemImage: Symbols.audio, value: audio.value) { audio }
                    }
                    if showsSubtitles {
                        choice(subtitles.title, systemImage: Symbols.subtitles, value: subtitles.value) { subtitles }
                    }
                }
                .padding(.horizontal, 20)
            }
            .scrollClipDisabled()
            .detailRow(EdgeInsets(top: 8, leading: 0, bottom: 8, trailing: 0))
            .sheet(isPresented: $searching) {
                if let version {
                    NavigationStack {
                        SubtitleSearchView(
                            display: server.display,
                            search: { try await server.session.playback.subtitleResults(version: version) },
                            download: { result in
                                let track = try await server.session.playback.addSubtitle(version: version, resultId: result.id)
                                downloaded(version.id, track)
                                model.chooseSubtitle(id: track.id)
                                searching = false
                            }
                        )
                        .toolbar {
                            ToolbarItem(placement: .cancellationAction) {
                                Button(String(localized: "Cancel")) { searching = false }
                            }
                        }
                    }
                }
            }
        }
    }

    private var versionName: String {
        version?.name ?? ""
    }

    private func choice<Content: View>(_ title: String, systemImage: String, value: String, @ViewBuilder content: () -> Content) -> some View {
        Menu {
            content().pickerStyle(.inline)
        } label: {
            HStack(spacing: KivoTokens.spacing * 1.5) {
                Image(systemName: systemImage)
                    .foregroundStyle(palette.textMuted)
                Text(value)
                    .lineLimit(1)
                Image(systemName: "chevron.up.chevron.down")
                    .font(.caption2.weight(.semibold))
                    .foregroundStyle(palette.textMuted)
            }
            .font(.subheadline)
            .foregroundStyle(palette.text)
            .padding(.horizontal, KivoTokens.spacing * 3)
            .frame(minHeight: 44)
            .frame(maxWidth: 220)
            .background(palette.surfaceRaised, in: Capsule())
        }
        .accessibilityLabel(Text(title))
        .accessibilityValue(Text(value))
    }
}
