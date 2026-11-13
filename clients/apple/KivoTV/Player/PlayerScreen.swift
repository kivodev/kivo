import AVKit
import KivoCore
import KivoUI
import SwiftUI

/// The player: the system's, presented over the app as the system's player
/// is, with its transport bar, its scrubbing, its Info panel and its skip
/// buttons, and Kivo's choices beside them. The transport bar's menus
/// choose the audio, subtitles, quality and version as the core's player
/// says; Skip Intro, Recap or Credits, and up next's Play now, are its
/// contextual actions; the chapters are its markers; Stream info is a tab
/// of its Info panel. Subtitle files the system can't draw, PGS and styled
/// ASS, are drawn over the video. A stopped play says why, with Try again,
/// another version where there is one, and Back. Menu leaves it as it
/// leaves the system's player, telling [closed].
struct PlayerScreen: View {
    let playback: VideoPlayback
    let closed: () -> Void
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        SystemPlayer(
            playback: playback,
            menus: PlayerMenus(playback),
            action: PlayerAction(playback),
            failure: failure,
            closed: closed
        )
        .frame(width: 0, height: 0)
        .onChange(of: scenePhase) { _, phase in
            switch phase {
            case .background: playback.stopOnExit()
            case .active: playback.restore()
            default: break
            }
        }
    }

    private var failure: PlayerFailureWords? {
        guard let state = playback.state, state.failure != nil else { return nil }
        return PlayerFailureWords(state.failure, display: playback.display)
    }
}

/// The transport bar's menus as the play has them: Audio with the
/// dialogue boost, Subtitles with Off and, where it can, Search subtitles,
/// Quality and, with several, Version. Each is left out with nothing to
/// choose; a download plays only what it kept.
struct PlayerMenus: Equatable {
    struct Choice: Equatable {
        let id: String
        let title: String
        var detail: String?
        let chosen: Bool
    }

    var audio: [Choice] = []
    /// Whether the dialogue boost is on, where the play offers it.
    var boost: Bool?
    var subtitles: [Choice] = []
    var searches = false
    var qualities: [Choice] = []
    var versions: [Choice] = []

    @MainActor
    init(_ playback: VideoPlayback) {
        guard let version = playback.version else { return }
        let audio = version.tracks.filter { $0.type == .audio && playback.offers($0) }
        if audio.count > 1 {
            self.audio = audio.map { Choice(id: $0.id, title: $0.pickerName.name, detail: $0.pickerName.note, chosen: $0.id == playback.audioTrackId) }
        }
        boost = playback.dialogueBoost?.enabled
        let subtitles = version.tracks.filter { $0.type == .subtitle && playback.offers($0) }
        searches = playback.searchesSubtitles
        if !subtitles.isEmpty || searches {
            self.subtitles = [Choice(id: SUBTITLES_OFF, title: String(localized: "Off"), chosen: playback.subtitle == SUBTITLES_OFF)]
                + subtitles.map { Choice(id: $0.id, title: $0.pickerName.name, detail: $0.pickerName.note, chosen: $0.id == playback.subtitle) }
        }
        if !playback.offline {
            let current = playback.quality
            qualities = playback.qualities.map { option in
                Choice(id: option.quality.key, title: playback.display.quality(option: option, sourceBps: version.bitrateBps), chosen: option.quality == current)
            }
            if let work = playback.work, work.versions.count > 1 {
                versions = work.versions.map { Choice(id: $0.id, title: $0.name, detail: Display.app.size(bytes: $0.sizeBytes), chosen: $0.id == version.id) }
            }
        }
    }
}

/// What the player's contextual action is: Skip, or up next's Play now
/// with Hide or Watch credits; nothing between them.
enum PlayerAction: Equatable {
    case none
    case skip(String)
    case upNext(atCredits: Bool)

    @MainActor
    init(_ playback: VideoPlayback) {
        if playback.upNext.shows, playback.next != nil {
            self = .upNext(atCredits: playback.upNext.atCredits)
        } else if let marker = playback.marker {
            self = .skip(skipLabel(marker))
        } else {
            self = .none
        }
    }
}

/// The system's player over the playback's engine, presented from where
/// this sits, its menus and contextual actions kept to [menus] and [action]
/// as they change, and a stopped play's [failure] said over it.
private struct SystemPlayer: UIViewControllerRepresentable {
    let playback: VideoPlayback
    let menus: PlayerMenus
    let action: PlayerAction
    let failure: PlayerFailureWords?
    let closed: () -> Void

    func makeUIViewController(context: Context) -> UIViewController {
        let controller = AVPlayerViewController()
        controller.player = playback.engine.player
        controller.allowsPictureInPicturePlayback = false
        // The subtitles are the transport bar's Subtitles menu's, which the core's player shows.
        controller.allowedSubtitleOptionLanguages = []
        controller.delegate = context.coordinator
        context.coordinator.attach(to: controller, playback: playback)
        return UIViewController()
    }

    func updateUIViewController(_ presenter: UIViewController, context: Context) {
        let coordinator = context.coordinator
        coordinator.closed = closed
        coordinator.show(menus: menus, action: action)
        coordinator.show(failure)
        // Once this is in the window, the player comes up over the app.
        DispatchQueue.main.async { coordinator.present(from: presenter) }
    }

    static func dismantleUIViewController(_ presenter: UIViewController, coordinator: Coordinator) {
        coordinator.detach()
    }

    func makeCoordinator() -> Coordinator { Coordinator() }

    @MainActor
    final class Coordinator: NSObject, AVPlayerViewControllerDelegate {
        var closed: () -> Void = {}
        private var controller: AVPlayerViewController?
        private var playback: VideoPlayback?
        private var menus: PlayerMenus?
        private var action: PlayerAction?
        private var overlay: SubtitleOverlayView?
        private var following: Task<Void, Never>?
        private var selection: NSObjectProtocol?
        private var presented = false
        private weak var alert: UIAlertController?

        /// Presents the player from [presenter], once.
        func present(from presenter: UIViewController) {
            guard !presented, let controller, presenter.view.window != nil else { return }
            presented = true
            presenter.present(controller, animated: true)
        }

        /// Says why the play stopped, over the player, with Try again,
        /// another version where there is one, and Back, which leaves it.
        func show(_ failure: PlayerFailureWords?) {
            guard let controller, let playback else { return }
            guard let failure else {
                alert?.dismiss(animated: true)
                return
            }
            guard alert == nil, presented else { return }
            let lines = [failure.sentence, failure.limit].compactMap { $0 } + failure.plays.map { play in
                play.clientName.map { "\(play.title) · \($0)" } ?? play.title
            }
            let alert = UIAlertController(title: failure.title, message: lines.joined(separator: "\n"), preferredStyle: .alert)
            alert.addAction(UIAlertAction(title: String(localized: "Try again"), style: .default) { _ in playback.retry() })
            for version in playback.work?.versions.filter({ $0.id != playback.version?.id }) ?? [] where (playback.work?.versions.count ?? 0) > 1 {
                alert.addAction(UIAlertAction(title: String(localized: "Play \(version.name)"), style: .default) { _ in playback.chooseVersion(version.id) })
            }
            alert.addAction(UIAlertAction(title: String(localized: "Back"), style: .cancel) { [weak controller] _ in
                controller?.dismiss(animated: true) { [weak self] in self?.closed() }
            })
            controller.present(alert, animated: true)
            self.alert = alert
        }

        /// Searches subtitles over the player, the up next countdown held while it shows.
        func search() {
            guard let controller, let playback else { return }
            playback.menuOpen = true
            let list = SubtitleSearchList(
                display: playback.display,
                search: { try await playback.searchSubtitles() },
                download: { try await playback.downloadSubtitle($0) }
            )
            .onDisappear { playback.menuOpen = false }
            controller.present(UIHostingController(rootView: list), animated: true)
        }

        func attach(to controller: AVPlayerViewController, playback: VideoPlayback) {
            self.controller = controller
            self.playback = playback
            let engine = playback.engine
            // The system's player fits the video in its view, as the overlay's rect does.
            let overlay = SubtitleOverlayView(engine: engine) { view in
                let size = engine.player.currentItem?.presentationSize ?? .zero
                return size.width > 0 ? AVMakeRect(aspectRatio: size, insideRect: view.bounds) : .zero
            }
            if let content = controller.contentOverlayView {
                overlay.frame = content.bounds
                overlay.autoresizingMask = [.flexibleWidth, .flexibleHeight]
                content.addSubview(overlay)
                let card = UIHostingController(rootView: PlayerOverlay(playback: playback))
                card.view.backgroundColor = .clear
                card.view.frame = content.bounds
                card.view.autoresizingMask = [.flexibleWidth, .flexibleHeight]
                content.addSubview(card.view)
            }
            self.overlay = overlay
            let info = UIHostingController(rootView: StreamInfoPanel(playback: playback))
            info.title = String(localized: "Stream info")
            controller.customInfoViewControllers = [info]
            following = Task { [weak self] in await self?.follow() }
            // The system's own Audio menu lists a file's audio tracks too: one chosen there plays through the core's player.
            selection = NotificationCenter.default.addObserver(forName: AVPlayerItem.mediaSelectionDidChangeNotification, object: nil, queue: .main) { [weak self] note in
                MainActor.assumeIsolated {
                    guard let item = note.object as? AVPlayerItem else { return }
                    self?.followAudio(of: item)
                }
            }
        }

        /// Plays the audio track [item] has selected as the core's choice, when it isn't already.
        private func followAudio(of item: AVPlayerItem) {
            guard let playback, item === playback.engine.player.currentItem else { return }
            Task {
                // The version lists the file's audio tracks in the file's order, as the item's options are.
                guard let position = await playback.engine.selectedAudioPosition(in: item),
                      let audio = playback.version?.tracks.filter({ $0.type == .audio }),
                      audio.indices.contains(position)
                else { return }
                let chosen = audio[position].id
                if chosen != playback.audioTrackId { playback.chooseAudio(chosen) }
            }
        }

        func detach() {
            if let selection { NotificationCenter.default.removeObserver(selection) }
            following?.cancel()
            overlay?.show(nil) { _ in Data() }
            if let controller, controller.presentingViewController != nil {
                controller.dismiss(animated: true)
            }
        }

        /// Follows what the system's player shows of the play outside
        /// the transport bar: the subtitle file over the video, and the
        /// chapters on the scrubber, as each play and part loads.
        private func follow() async {
            while !Task.isCancelled, let playback {
                let item = playback.engine.player.currentItem
                overlay?.show(playback.externalSubtitle) { try await playback.read($0) }
                if let item, item.navigationMarkerGroups.isEmpty, !playback.chapters.isEmpty {
                    item.navigationMarkerGroups = [Self.chapters(playback.chapters)]
                }
                await withCheckedContinuation { continuation in
                    withObservationTracking {
                        _ = playback.externalSubtitle
                        _ = playback.chapters
                        _ = playback.state?.play
                    } onChange: {
                        continuation.resume()
                    }
                }
            }
        }

        /// The play's chapters as the scrubber's markers.
        private static func chapters(_ chapters: [Chapter]) -> AVNavigationMarkersGroup {
            let markers = chapters.map { chapter in
                let title = AVMutableMetadataItem()
                title.identifier = .commonIdentifierTitle
                title.value = (chapter.title ?? "") as NSString
                title.extendedLanguageTag = "und"
                let range = CMTimeRange(
                    start: CMTime(value: Int64(chapter.startMs), timescale: 1000),
                    end: CMTime(value: Int64(chapter.endMs), timescale: 1000)
                )
                return AVTimedMetadataGroup(items: [title], timeRange: range)
            }
            return AVNavigationMarkersGroup(title: nil, timedNavigationMarkers: markers)
        }

        /// Sets the menus and the contextual action, each only as it changes, so an open menu stays as it is.
        func show(menus: PlayerMenus, action: PlayerAction) {
            guard let controller, let playback else { return }
            if menus != self.menus {
                self.menus = menus
                controller.transportBarCustomMenuItems = Self.items(menus, playback: playback, search: { [weak self] in self?.search() })
            }
            if action != self.action {
                self.action = action
                switch action {
                case .none:
                    controller.contextualActions = []
                case let .skip(label):
                    controller.contextualActions = [UIAction(title: label) { _ in playback.skipPastMarker() }]
                case let .upNext(atCredits):
                    // Play now last, where the system puts focus.
                    controller.contextualActions = [
                        UIAction(title: atCredits ? String(localized: "Watch credits") : String(localized: "Hide")) { _ in playback.hideUpNext() },
                        UIAction(title: String(localized: "Play now"), image: UIImage(systemName: Symbols.play)) { _ in playback.playNextNow() },
                    ]
                }
            }
        }

        private static func items(_ menus: PlayerMenus, playback: VideoPlayback, search: @escaping () -> Void) -> [UIMenuElement] {
            func choices(_ choices: [PlayerMenus.Choice], choose: @escaping (String) -> Void) -> [UIMenuElement] {
                choices.map { choice in
                    UIAction(title: choice.title, subtitle: choice.detail, state: choice.chosen ? .on : .off) { _ in choose(choice.id) }
                }
            }
            var items: [UIMenuElement] = []
            if !menus.audio.isEmpty || menus.boost != nil {
                var audio = choices(menus.audio) { playback.chooseAudio($0) }
                if let boost = menus.boost {
                    audio.append(UIMenu(options: .displayInline, children: [
                        UIAction(title: String(localized: "Dialogue boost"), state: boost ? .on : .off) { _ in playback.boostDialogue(!boost) },
                    ]))
                }
                items.append(UIMenu(title: String(localized: "Audio"), image: UIImage(systemName: Symbols.audio), children: audio))
            }
            if !menus.subtitles.isEmpty {
                var subtitles = choices(menus.subtitles) { playback.chooseSubtitle($0) }
                if menus.searches {
                    subtitles.append(UIMenu(options: .displayInline, children: [
                        UIAction(title: String(localized: "Search subtitles…"), image: UIImage(systemName: "magnifyingglass")) { _ in search() },
                    ]))
                }
                items.append(UIMenu(title: String(localized: "Subtitles"), image: UIImage(systemName: Symbols.subtitles), children: subtitles))
            }
            var settings: [UIMenuElement] = []
            if !menus.qualities.isEmpty {
                settings.append(UIMenu(title: String(localized: "Quality"), children: choices(menus.qualities) { key in
                    if let option = playback.qualities.first(where: { $0.quality.key == key }) { playback.chooseQuality(option) }
                }))
            }
            if !menus.versions.isEmpty {
                settings.append(UIMenu(title: String(localized: "Version"), children: choices(menus.versions) { playback.chooseVersion($0) }))
            }
            if !settings.isEmpty {
                items.append(UIMenu(title: String(localized: "Playback"), image: UIImage(systemName: Symbols.playback), children: settings))
            }
            return items
        }

        // MARK: AVPlayerViewControllerDelegate

        /// Menu left the player.
        nonisolated func playerViewControllerDidEndDismissalTransition(_ playerViewController: AVPlayerViewController) {
            MainActor.assumeIsolated { closed() }
        }

        /// The core's player follows the transport bar, for how long Skip shows.
        nonisolated func playerViewController(
            _ playerViewController: AVPlayerViewController,
            willTransitionToVisibilityOfTransportBar visible: Bool,
            with coordinator: any AVPlayerViewControllerAnimationCoordinator
        ) {
            MainActor.assumeIsolated {
                if visible { playback?.input() } else { playback?.hideControls() }
            }
        }
    }
}

/// Over the video, under the system's controls: a spinner until the play
/// starts, and up next's next episode while its card shows.
private struct PlayerOverlay: View {
    let playback: VideoPlayback
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        ZStack(alignment: .bottomTrailing) {
            Color.clear
            if playback.play == nil, playback.state?.failure == nil {
                ProgressView()
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
            if playback.upNext.shows, let next = playback.next {
                UpNextCard(next: next, upNext: playback.upNext)
                    // Above the contextual actions, at the screen's safe edge.
                    .padding(.trailing, 90)
                    .padding(.bottom, 200)
                    .transition(.opacity)
            }
        }
        .animation(.easeInOut(duration: 0.3), value: playback.upNext.shows)
    }
}

/// Up next's next episode: its still, its countdown or "Still watching?",
/// and its code and title.
private struct UpNextCard: View {
    let next: WorkDetail
    let upNext: UpNext
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing * 3) {
            ArtworkImage(next.images.still ?? next.images.backdrop, title: next.title, pointWidth: 400)
                .frame(width: 400, height: 225)
                .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
            Text(upNextLabel(upNext))
                .kivoStyle(KivoTokens.TextSize.sm, weight: upNext.phase == .stillWatching ? KivoTokens.Weight.semibold : KivoTokens.Weight.normal)
                .foregroundStyle(upNext.phase == .stillWatching ? palette.onPlayer : palette.onPlayerMuted)
            Text(next.episode.map { "\(Display.app.episodeCode(episode: $0)) · \(next.title)" } ?? next.title)
                .kivoStyle(KivoTokens.TextSize.base, weight: KivoTokens.Weight.semibold)
                .foregroundStyle(palette.onPlayer)
                .lineLimit(2)
        }
        .frame(width: 400, alignment: .leading)
        .padding(KivoTokens.spacing * 6)
        .background(palette.playerPanel, in: RoundedRectangle(cornerRadius: KivoTokens.Radius.lg))
    }
}

/// Stream info, a tab of the Info panel: each line's label and value, and why where it isn't direct.
private struct StreamInfoPanel: View {
    let playback: VideoPlayback
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        let rows = playback.streamInfo.map {
            StreamInfoRow.rows($0, display: playback.display, version: playback.version, subtitle: playback.subtitle, offline: playback.offline, quality: playback.quality)
        } ?? []
        Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: KivoTokens.spacing * 10, verticalSpacing: KivoTokens.spacing * 3) {
            ForEach(rows) { row in
                GridRow {
                    Text(row.label)
                        .foregroundStyle(palette.textMuted)
                    VStack(alignment: .leading, spacing: KivoTokens.spacing) {
                        Text(row.value)
                        if let why = row.why {
                            Text("Why: \(why)").foregroundStyle(palette.textMuted)
                        }
                        if let note = row.note {
                            Text(note).foregroundStyle(palette.textMuted)
                        }
                    }
                }
            }
        }
        .font(.callout)
        .padding(.horizontal, KivoTokens.spacing * 20)
        .padding(.vertical, KivoTokens.spacing * 10)
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
