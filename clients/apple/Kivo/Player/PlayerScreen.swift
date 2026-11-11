import KivoCore
import KivoUI
import SwiftUI

/// The player's screen: the video, the subtitle overlay, and Kivo's own
/// controls, in the layout for how the phone is held (``PlayerLayout``).
/// A tap shows or hides the controls, a double tap on the video's left or
/// right third skips, and a pinch fills or fits it.
struct PlayerScreen: View {
    let model: PlayerModel
    @Environment(\.kivoPalette) private var palette
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.dismiss) private var dismiss
    @State private var burst: SkipBurst?
    /// The window's size, safe areas and all, which picks the layout.
    @State private var size = CGSize.zero
    /// Whether an unfolded phone is half open, as on a table.
    @State private var halfOpen = false
    /// The screen's safe area, which on an iPhone Duo keeps a rail down one side.
    @State private var insets = EdgeInsets()

    var body: some View {
        @Bindable var model = model
        let layout = PlayerLayout(size, halfOpen: halfOpen)
        let controlsShown = model.controlsVisible
        // A phone draws its own top row, which the system's bar lays out
        // unpredictably on an unfolded phone; an iPad keeps the system's,
        // clear of the window's controls.
        let pad = UIDevice.current.userInterfaceIdiom == .pad
        // The title ends short of the middle where the fold crosses the top
        // row, on an unfolded phone in landscape, and of the bar's buttons
        // in an iPad's window, however narrow.
        let titleWidth = pad || Orientation.unfolded(size) && size.width > size.height ? max(120, size.width / 2 - 80 - insets.leading) : .infinity
        let rail = pad ? nil : RouteRail.Side(insets)
        // Clear of the screen's top edge where nothing else keeps it so.
        let top = max(0, PlayerGlyph.edge - insets.top)
        NavigationStack {
            ZStack {
                Color.black
                    .ignoresSafeArea()
                    .onGeometryChange(for: CGSize.self) { $0.size } action: { size = $0 }
                    .onGeometryChange(for: EdgeInsets.self) { $0.safeAreaInsets } action: { insets = $0 }
                    .background(HingeReader(halfOpen: $halfOpen))
                    // A tap off the video and its controls shows or hides them.
                    .contentShape(Rectangle())
                    .onTapGesture { model.toggleControls() }
                switch layout {
                case .overlay:
                    video
                        .ignoresSafeArea()
                    if controlsShown {
                        PlayerControls(model: model, topRow: !pad, routes: rail == nil, top: top, titleWidth: titleWidth, close: { dismiss() })
                            .transition(.opacity)
                        if let rail {
                            RouteRail(model: model, side: rail, insets: insets, bottom: PlayerControls.bottom)
                                .transition(.opacity)
                        }
                    }
                    corner(clearing: 84)
                case .upright:
                    VStack(spacing: 0) {
                        PlayerTopRow(model: model, titled: false, routes: rail == nil, close: { dismiss() })
                            .padding(.horizontal, PlayerGlyph.edge)
                            .padding(.top, top)
                            .hidden(!controlsShown)
                        Spacer(minLength: 0)
                        framedVideo
                        Spacer(minLength: 0)
                        VStack(spacing: KivoTokens.spacing * 3) {
                            if model.upNext.shows, let next = model.next {
                                upNextRow(next, narrow: true)
                                    .transition(.opacity)
                            }
                            UprightControls(model: model)
                                .keepsControlsUp(model)
                                .hidden(!controlsShown)
                        }
                        // On the top row's edges.
                        .padding(.horizontal, PlayerGlyph.edge)
                        .padding(.bottom, UprightControls.bottom)
                    }
                    if let rail {
                        RouteRail(model: model, side: rail, insets: insets, bottom: UprightControls.bottom)
                            .hidden(!controlsShown)
                    }
                case .unfoldedUpright:
                    VStack(spacing: 0) {
                        PlayerTopRow(model: model, routes: rail == nil, titleWidth: titleWidth, close: { dismiss() })
                            .padding(.horizontal, PlayerGlyph.edge)
                            .padding(.top, top)
                            .hidden(!controlsShown)
                        Spacer(minLength: 0)
                        framedVideo
                        Spacer(minLength: 0)
                        VStack(spacing: KivoTokens.spacing * 4) {
                            if let next = model.next {
                                // Up next, with the controls; and while it shows, without them.
                                upNextRow(next)
                                    .hidden(!controlsShown && !model.upNext.shows)
                            }
                            ControlBar(model: model)
                                .keepsControlsUp(model)
                                .hidden(!controlsShown)
                        }
                        .padding(.horizontal, PlayerGlyph.edge)
                        .padding(.bottom, UprightControls.bottom)
                    }
                    if let rail {
                        RouteRail(model: model, side: rail, insets: insets, bottom: UprightControls.bottom)
                            .hidden(!controlsShown)
                    }
                case .tabletop:
                    tabletop
                }
                status(controlsShown: controlsShown)
            }
            .toolbar {
                if pad, controlsShown {
                    PlayerTopBar(model: model, titleWidth: titleWidth, close: { dismiss() })
                }
            }
            .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
            // The bar's buttons are white over the video, as the other controls are.
            .tint(palette.onPlayer)
            .toolbarVisibility(pad && controlsShown ? .visible : .hidden, for: .navigationBar)
            .navigationBarTitleDisplayMode(.inline)
        }
        .animation(.easeInOut(duration: 0.3), value: controlsShown)
        .statusBarHidden(!controlsShown)
        .persistentSystemOverlays(controlsShown ? .automatic : .hidden)
        .preferredColorScheme(.dark)
        // Turned upright by the Cast remote it took over from, it follows the phone again.
        .onAppear {
            Orientation.follow()
            model.screenShown()
        }
        .onChange(of: scenePhase) { _, phase in
            switch phase {
            case .background: model.background()
            case .active: model.foreground()
            default: break
            }
        }
    }

    /// The video, the subtitles drawn over it, and its taps, double taps and pinches.
    private var video: some View {
        ZStack {
            PlayerSurface(view: model.surface)
            if let externalSubtitle = model.externalSubtitle {
                SubtitleOverlay(externalSubtitle: externalSubtitle, model: model)
            }
            VideoGestures(model: model, burst: $burst)
                .onContinuousHover { phase in
                    if case .active = phase { model.input() }
                }
            if let burst {
                SkipRipple(burst: burst)
                    .allowsHitTesting(false)
            }
        }
    }

    /// The video in a 16:9 frame clear of the controls, with Skip Intro in its corner.
    private var framedVideo: some View {
        ZStack {
            video
            corner(clearing: 0, upNext: false)
        }
        .aspectRatio(16 / 9, contentMode: .fit)
        .clipped()
    }

    /// Half open on a table: the video above the fold, the controls below it.
    private var tabletop: some View {
        // Measured on the whole screen, which folds across its middle.
        GeometryReader { screen in
            let insets = screen.safeAreaInsets
            let fold = screen.size.height / 2
            VStack(spacing: 0) {
                framedVideo
                    .frame(maxWidth: .infinity)
                    .frame(height: max(0, fold - insets.top))
                    .padding(.top, insets.top)
                TabletopControls(model: model, width: screen.size.width - TabletopControls.margin * 2, close: { dismiss() })
                    .padding(.horizontal, TabletopControls.margin)
                    .padding(.top, TabletopControls.foldClearance)
                    .layoutPriority(1)
                    .keepsControlsUp(model)
                    // Dark while it plays: a tap brings them up, and they fade away
                    // again; up while up next shows in them.
                    .hidden(!model.controlsVisible && !model.upNext.shows)
                Spacer(minLength: insets.bottom)
            }
        }
        .ignoresSafeArea()
    }

    /// The next episode under the video, which takes a tap that misses its buttons as use.
    private func upNextRow(_ next: WorkDetail, narrow: Bool = false) -> some View {
        UpNextRow(next: next, upNext: model.upNext, play: model.playNextNow, hide: model.hideUpNext, narrow: narrow)
            .keepsControlsUp(model)
    }

    /// Skip Intro, Recap or Credits, and up next unless the controls under
    /// the video carry it, bottom right, [clearing] the controls when they show.
    private func corner(clearing controls: CGFloat, upNext: Bool = true) -> some View {
        VStack {
            Spacer()
            HStack {
                Spacer()
                if upNext, model.upNext.shows, let next = model.next {
                    UpNextCard(next: next, upNext: model.upNext, play: model.playNextNow, hide: model.hideUpNext)
                } else if let marker = model.marker {
                    Button(skipLabel(marker)) {
                        model.skipPastMarker()
                    }
                    .buttonStyle(PlayerPillStyle(prominent: false))
                }
            }
            .padding(.horizontal, PlayerGlyph.edge)
            .padding(.bottom, model.controlsVisible ? max(controls, KivoTokens.spacing * 4) : KivoTokens.spacing * 4)
        }
        .animation(.easeInOut(duration: 0.2), value: model.controlsVisible)
    }

    private func skipLabel(_ marker: SkipMarker) -> String {
        switch marker.kind {
        case .credits: String(localized: "Skip Credits")
        case .recap: String(localized: "Skip Recap")
        default: String(localized: "Skip Intro")
        }
    }

    /// Loading, buffering, or the failure itself. The spinner takes the play
    /// button's place while the controls show.
    @ViewBuilder private func status(controlsShown: Bool) -> some View {
        if let state = model.state {
            switch state.status {
            case .error:
                PlayerErrorView(model: model, state: state, close: { dismiss() })
            case .loading, .buffering, .seeking:
                if !controlsShown { Spinner().allowsHitTesting(false) }
            case .idle, .playing:
                EmptyView()
            }
        }
    }
}

/// How the player lays out in a window of a size.
private enum PlayerLayout {
    /// In landscape, and on an iPad: the controls over the video.
    case overlay
    /// A phone upright: the video, then the controls in a card under it.
    case upright
    /// An unfolded phone upright and flat: the video in the middle, the
    /// next episode and the control bar under it.
    case unfoldedUpright
    /// An unfolded phone upright and half open on a table: the video above
    /// the fold, larger controls below it.
    case tabletop

    @MainActor init(_ size: CGSize, halfOpen: Bool) {
        if size.height <= size.width || UIDevice.current.userInterfaceIdiom != .phone {
            self = .overlay
        } else if !Orientation.unfolded(size) {
            self = .upright
        } else {
            self = halfOpen ? .tabletop : .unfoldedUpright
        }
    }
}

/// Reads the phone's hinge: whether it's half open, as on a table. Never
/// before iOS 27.1, which first tells.
private struct HingeReader: UIViewRepresentable {
    @Binding var halfOpen: Bool

    func makeUIView(context: Context) -> UIView {
        let view = UIView()
        view.isUserInteractionEnabled = false
        if #available(iOS 27.1, *) {
            let halfOpen = $halfOpen
            view.addInteraction(UIHingeInteraction { _, update in
                halfOpen.wrappedValue = update.hinge?.status == .partiallyOpen
            })
        }
        return view
    }

    func updateUIView(_ view: UIView, context: Context) {}
}

private extension View {
    /// Faded out and out of reach, VoiceOver's too, while [hidden], as the controls are while it plays.
    func hidden(_ hidden: Bool) -> some View {
        opacity(hidden ? 0 : 1)
            .allowsHitTesting(!hidden)
            .accessibilityHidden(hidden)
    }
}

/// A run of double-tap skips on one side, which the ripple totals.
struct SkipBurst: Equatable {
    enum Side {
        case back
        case forward
    }

    let side: Side
    var count: Int
    var at: Date

    var seconds: Int {
        count * Int((side == .back ? SKIP_BACK_MS : SKIP_FORWARD_MS) / 1000)
    }
}

/// The taps, double taps and pinches on the video itself.
private struct VideoGestures: View {
    let model: PlayerModel
    @Binding var burst: SkipBurst?

    var body: some View {
        GeometryReader { geometry in
            Color.clear
                .contentShape(Rectangle())
                .gesture(
                    SpatialTapGesture(count: 2)
                        .onEnded { value in doubleTap(at: value.location.x, width: geometry.size.width) }
                        .exclusively(before: SpatialTapGesture()
                            .onEnded { value in tap(at: value.location.x, width: geometry.size.width) })
                )
                .simultaneousGesture(
                    MagnifyGesture().onEnded { value in
                        if value.magnification > 1.15 { model.fill = true }
                        if value.magnification < 0.87 { model.fill = false }
                    }
                )
                .accessibilityHidden(true)
        }
        .sensoryFeedback(.impact(weight: .light), trigger: burst?.count) { _, new in new != nil }
        .sensoryFeedback(.selection, trigger: model.fill)
    }

    private func side(at x: CGFloat, width: CGFloat) -> SkipBurst.Side? {
        if x < width / 3 { return .back }
        if x > width * 2 / 3 { return .forward }
        return nil
    }

    /// A tap shows or hides the controls; during a run of skips, a tap on the same side adds one.
    private func tap(at x: CGFloat, width: CGFloat) {
        if let burst, Date().timeIntervalSince(burst.at) < 1, side(at: x, width: width) == burst.side {
            skip(burst.side)
        } else {
            model.toggleControls()
        }
    }

    private func doubleTap(at x: CGFloat, width: CGFloat) {
        guard let side = side(at: x, width: width) else {
            model.toggleControls()
            return
        }
        skip(side)
    }

    private func skip(_ side: SkipBurst.Side) {
        let now = Date()
        if var current = burst, current.side == side, now.timeIntervalSince(current.at) < 1 {
            current.count += 1
            current.at = now
            burst = current
        } else {
            burst = SkipBurst(side: side, count: 1, at: now)
        }
        model.nudge(by: side == .back ? -SKIP_BACK_MS : SKIP_FORWARD_MS)
        let shown = burst
        Task {
            try? await Task.sleep(for: .milliseconds(900))
            if burst == shown { burst = nil }
        }
    }
}

/// The double tap's feedback: a soft half-circle on that side with the
/// arrows and the total, "−20s" or "+30s".
private struct SkipRipple: View {
    let burst: SkipBurst

    var body: some View {
        GeometryReader { geometry in
            let back = burst.side == .back
            ZStack {
                Ellipse()
                    .fill(.white.opacity(0.12))
                    .frame(width: geometry.size.width * 0.5, height: geometry.size.height * 1.3)
                    .offset(x: (back ? -1 : 1) * geometry.size.width * 0.45)
                VStack(spacing: KivoTokens.spacing * 2) {
                    Image(systemName: back ? "backward.fill" : "forward.fill")
                        .font(.system(size: 28, weight: .semibold))
                    Text(back ? "−\(burst.seconds)s" : "+\(burst.seconds)s")
                        .font(.headline.monospacedDigit())
                }
                .foregroundStyle(.white)
                .shadow(color: .black.opacity(0.4), radius: 4)
                .offset(x: (back ? -1 : 1) * geometry.size.width * 0.36)
            }
            .frame(width: geometry.size.width, height: geometry.size.height)
            .clipped()
        }
        .transition(.opacity)
        .animation(.easeOut(duration: 0.2), value: burst.count)
    }
}

/// A calm spinner over the video.
struct Spinner: View {
    var body: some View {
        ProgressView()
            .controlSize(.large)
            .tint(.white)
            .frame(width: 64, height: 64)
            .background(.black.opacity(0.35), in: Circle())
    }
}

/// A pill on the video: white and solid for the main choice, glass otherwise.
struct PlayerPillStyle: ButtonStyle {
    let prominent: Bool

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.subheadline.weight(.semibold))
            .foregroundStyle(prominent ? Color.black : Color.white)
            .padding(.horizontal, KivoTokens.spacing * 4)
            .frame(minHeight: 44)
            .contentShape(Capsule())
            .background {
                if prominent { Capsule().fill(.white) }
            }
            .playerGlass(in: Capsule(), when: !prominent)
            .opacity(configuration.isPressed ? 0.7 : 1)
    }
}

/// Up next: the next episode's still, title and countdown,
/// with Play now and Hide, or Watch credits when it came at the credits;
/// no countdown with the user's autoplay off; after three without input,
/// "Still watching?".
private struct UpNextCard: View {
    let next: WorkDetail
    let upNext: UpNext
    let play: () -> Void
    let hide: () -> Void
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        VStack(alignment: .leading, spacing: KivoTokens.spacing * 2) {
            ArtworkImage(next.images.still ?? next.images.backdrop, title: next.title, pointWidth: 240)
                .frame(width: 240, height: 135)
                .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.sm))
            let stillWatching = upNext.phase == .stillWatching
            Text(upNextLabel(upNext))
                .font(.caption.weight(stillWatching ? .semibold : .regular))
                .foregroundStyle(stillWatching ? palette.onPlayer : palette.onPlayerMuted)
            Text(next.episode.map { "\(Display.app.episodeCode(episode: $0)) · \(next.title)" } ?? next.title)
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(palette.onPlayer)
                .lineLimit(2)
            HStack {
                Button(String(localized: "Play now"), action: play).buttonStyle(PlayerPillStyle(prominent: true))
                Button(upNext.atCredits ? String(localized: "Watch credits") : String(localized: "Hide"), action: hide)
                    .buttonStyle(PlayerPillStyle(prominent: false))
            }
        }
        .padding(KivoTokens.spacing * 3)
        .frame(width: KivoTokens.Layout.upNextWidth * 0.8)
        .background(palette.playerPanel, in: RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
    }
}

/// Playback stopped: what failed, the server's reason in
/// words, Try again, Play another version where there is one, and Back.
private struct PlayerErrorView: View {
    let model: PlayerModel
    let state: PlayerState
    let close: () -> Void
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        let words = self.words
        ScrollView {
            VStack(spacing: KivoTokens.spacing * 3) {
                Image(systemName: "exclamationmark.triangle")
                    .font(.system(size: 32, weight: .semibold))
                    .foregroundStyle(palette.onPlayerMuted)
                Text(words.title).font(.title3.weight(.semibold))
                Text(words.sentence).foregroundStyle(palette.onPlayerMuted)
                if let limit = words.limit {
                    Text(limit).font(.footnote).foregroundStyle(palette.onPlayerMuted)
                }
                ForEach(words.plays, id: \.id) { play in
                    Text(play.clientName.map { "\(play.title) · \($0)" } ?? play.title).font(.footnote)
                }
                HStack {
                    Button(String(localized: "Try again"), action: model.retry).buttonStyle(PlayerPillStyle(prominent: true))
                    if let work = model.work, work.versions.count > 1 {
                        Menu(String(localized: "Play another version")) {
                            ForEach(work.versions, id: \.id) { version in
                                Button(version.name) { model.chooseVersion(version.id) }
                            }
                        }
                        .buttonStyle(PlayerPillStyle(prominent: false))
                    }
                    Button(String(localized: "Back"), action: close).buttonStyle(PlayerPillStyle(prominent: false))
                }
                .padding(.top, KivoTokens.spacing * 2)
            }
            .multilineTextAlignment(.center)
            .frame(maxWidth: 480)
            .padding(KivoTokens.spacing * 6)
            .frame(maxWidth: .infinity)
        }
        .scrollBounceBehavior(.basedOnSize)
        .defaultScrollAnchor(.center)
        .foregroundStyle(palette.onPlayer)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(palette.playerScrim.ignoresSafeArea())
    }

    /// The failure in words, the capability a file failed, and for the stream limit what plays where.
    private var words: (title: String, sentence: String, limit: String?, plays: [PlaySummary]) {
        guard let failure = state.failure else {
            return (String(localized: "Couldn't play this."), model.display.problem(code: "internal"), nil, [])
        }
        let sentence = model.display.failure(failure: failure)
        switch onEnum(of: failure) {
        case .ended:
            return (String(localized: "Playback was stopped."), sentence, nil, [])
        case let .problem(problem):
            let title = problem.knownCode == .streamLimit ? String(localized: "You're at your stream limit.") : String(localized: "Couldn't play this.")
            return (title, sentence, problem.mismatch.map(StreamInfoView.words), problem.plays ?? [])
        }
    }
}

