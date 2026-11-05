import KivoCore
import KivoUI
import SwiftUI

/// The controls over the video, in landscape: on a phone the top row
/// (``PlayerTopRow``), on an iPad the system's top bar (``PlayerTopBar``);
/// and one glass bar along the bottom with back 10, play or pause and
/// forward 30, the scrub bar between the time played and the time left,
/// and Fill, Audio & Subtitles and Playback settings, so nothing covers the
/// middle of the picture, nor an unfolded phone's fold. The two rows share
/// their margins. Soft gradients at the top and bottom keep them legible. A
/// tap that lands in the bar but misses a button keeps the controls up
/// rather than hiding them.
struct PlayerControls: View {
    let model: PlayerModel
    /// The top row, on a phone; an iPad has the system's top bar.
    var topRow = true
    /// Cast, AirPlay and Picture in Picture in the top row, unless a ``RouteRail`` has them.
    var routes = true
    /// From the safe area's top to the top row.
    var top: CGFloat = PlayerGlyph.edge
    var titleWidth: CGFloat = 320
    let close: () -> Void

    /// From the safe area's foot to the bar.
    static let bottom = KivoTokens.spacing * 3

    var body: some View {
        ZStack {
            VStack(spacing: 0) {
                LinearGradient(colors: [.black.opacity(0.5), .clear], startPoint: .top, endPoint: .bottom)
                    .frame(height: 110)
                Spacer(minLength: 0)
                LinearGradient(colors: [.clear, .black.opacity(0.6)], startPoint: .top, endPoint: .bottom)
                    .frame(height: 130)
            }
            .ignoresSafeArea()
            .allowsHitTesting(false)

            VStack(spacing: 0) {
                if topRow {
                    PlayerTopRow(model: model, routes: routes, titleWidth: titleWidth, close: close)
                        .padding(.top, top)
                }
                Spacer(minLength: 0)
                ControlBar(model: model, fill: true)
                    .keepsControlsUp(model)
            }
            .padding(.horizontal, PlayerGlyph.edge)
            .padding(.bottom, Self.bottom)
        }
    }
}

/// The controls under the video on a phone held upright, in one glass
/// card: the title, the scrub bar, and back 10, play or
/// pause and forward 30 with Audio & Subtitles and Playback settings.
struct UprightControls: View {
    let model: PlayerModel

    /// From the safe area's foot to the controls under the video.
    static let bottom = KivoTokens.spacing * 2

    var body: some View {
        // Spaced by what shows, the track and the glyphs, not by their
        // 44-point targets, which overlap the gaps between them.
        VStack(alignment: .leading, spacing: 0) {
            PlayerTitle(model: model)
            ScrubBar(model: model)
            HStack(spacing: 0) {
                Transport(model: model)
                Spacer(minLength: 0)
                PanelButtons(model: model)
            }
            // The glyphs, not their 44-point targets, line up with the text.
            .padding(.horizontal, -PlayerGlyph.inset)
            .padding(.top, -PlayerGlyph.inset)
        }
        .padding([.horizontal, .top], KivoTokens.spacing * 4)
        .padding(.bottom, KivoTokens.spacing * 4 - PlayerGlyph.inset)
        .playerGlass(in: RoundedRectangle(cornerRadius: 28, style: .continuous), interactive: false)
        .buttonStyle(PlayerIconStyle())
        .foregroundStyle(.white)
    }
}

/// An unfolded phone half open on a table: the half lying flat holds the
/// controls as tiles on a six-column grid, to reach for across a table.
/// Close, the title, Cast where there's a device, AirPlay and Picture in
/// Picture; back 10, play or pause and forward 30, taking the room left;
/// the scrub bar; and Audio & Subtitles, Playback, and an episode's next
/// one or a movie's Fill, which while up next shows give way to it: the
/// next episode, Play now, and Hide or Watch credits, in the same tiles.
struct TabletopControls: View {
    let model: PlayerModel
    /// The grid's width, between the margins.
    let width: CGFloat
    let close: () -> Void
    @Environment(\.kivoPalette) private var palette

    /// From the screen's edges, and between tiles.
    static let margin: CGFloat = 9
    /// From the fold to the first row: the crease bends well past its line.
    static let foldClearance: CGFloat = 56
    private let gap = TabletopControls.margin

    var body: some View {
        let column = (width - gap * 5) / 6
        let span = { (columns: CGFloat) in column * columns + gap * (columns - 1) }
        let cast = model.cast.buttonShown
        let paused = model.state?.paused != false
        VStack(spacing: gap) {
            HStack(spacing: gap) {
                tile(String(localized: "Close"), action: close) {
                    Image(systemName: "xmark")
                }
                .frame(width: span(1))
                .accessibilityIdentifier("Close")
                PlayerTitle(model: model)
                    .padding(.horizontal, KivoTokens.spacing * 4)
                    .frame(width: span(cast ? 2 : 3), alignment: .leading)
                    .frame(maxHeight: .infinity)
                    .playerTile()
                if cast {
                    tile(String(localized: "Cast"), action: model.cast.buttonTapped) {
                        CastGlyph(connection: model.cast.connection)
                            .frame(width: 24, height: 24)
                            .foregroundStyle(model.cast.connection == .connected ? palette.accent : .white)
                    }
                    .frame(width: span(1))
                    .accessibilityIdentifier("cast")
                }
                AirPlayButton(active: model.airPlaying)
                    .frame(width: span(1))
                    .playerTile()
                tile(String(localized: "Picture in Picture"), action: model.startPictureInPicture) {
                    Image(systemName: "pip.enter")
                }
                .frame(width: span(1))
            }
            .frame(height: 77)
            HStack(spacing: gap) {
                tile(String(localized: "Back 10 seconds")) {
                    model.skip(by: -Markers.shared.BACK_MS)
                } label: {
                    Image(systemName: "gobackward.10")
                }
                if model.waiting {
                    ProgressView()
                        .controlSize(.large)
                        .tint(.white)
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                        .playerTile()
                } else {
                    tile(paused ? String(localized: "Play") : String(localized: "Pause"), action: model.togglePlay) {
                        Image(systemName: paused ? "play.fill" : "pause.fill")
                            .contentTransition(.symbolEffect(.replace))
                    }
                    .accessibilityIdentifier(paused ? "Play" : "Pause")
                }
                tile(String(localized: "Forward 30 seconds")) {
                    model.skip(by: Markers.shared.FORWARD_MS)
                } label: {
                    Image(systemName: "goforward.30")
                }
            }
            .font(PlayerGlyph.transportTile)
            .frame(minHeight: 72, maxHeight: 143)
            .sensoryFeedback(.impact(weight: .light), trigger: paused) { _, _ in model.controlsVisible }
            ScrubBar(model: model)
                .padding(.horizontal, KivoTokens.spacing * 4)
                .frame(height: 51)
                .playerTile()
            HStack(spacing: gap) {
                // Not over an open panel, which hangs from its tile.
                if model.upNext.shows, model.sheet == nil, let next = model.next {
                    upNext(next)
                } else {
                    choices
                }
            }
            .frame(height: 51)
        }
        .font(PlayerGlyph.controlTile)
        .foregroundStyle(.white)
    }

    /// Audio & Subtitles, Playback, and the next episode or Fill.
    @ViewBuilder private var choices: some View {
                labelled(String(localized: "Subtitles"), systemImage: "captions.bubble") {
                    model.sheet = .tracks
                }
                .accessibilityLabel(Text("Audio & Subtitles"))
                .accessibilityIdentifier("tracks")
                .modifier(PlayerPanel(model: model, sheet: .tracks))
                labelled(String(localized: "Playback"), systemImage: "gearshape") {
                    model.sheet = .settings
                }
                .accessibilityLabel(Text("Playback Settings"))
                .accessibilityIdentifier("settings")
                .modifier(PlayerPanel(model: model, sheet: .settings))
                if model.next != nil {
                    labelled(String(localized: "Next Episode"), systemImage: "forward.end", action: model.playNextNow)
                } else {
                    labelled(model.fill ? String(localized: "Fit") : String(localized: "Fill"), systemImage: model.fillGlyph, action: model.toggleFill)
                        .accessibilityIdentifier("fill")
                }
    }

    /// Up next across the bottom row: what's next and its countdown, Play now, and Hide or Watch credits.
    @ViewBuilder private func upNext(_ next: WorkDetail) -> some View {
        let upNext = model.upNext
        let stillWatching = upNext.phase == .stillWatching
        VStack(alignment: .leading, spacing: 1) {
            Text(upNextLabel(upNext))
                .font(.caption.weight(stillWatching ? .semibold : .regular))
                .foregroundStyle(stillWatching ? palette.onPlayer : palette.onPlayerMuted)
            Text(next.episode.map { "\(Display.app.episodeCode(episode: $0)) · \(next.title)" } ?? next.title)
                .font(.system(size: 15, weight: .medium))
        }
        .lineLimit(1)
        .padding(.horizontal, KivoTokens.spacing * 4)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .playerTile()
        .accessibilityElement(children: .combine)
        labelled(String(localized: "Play now"), systemImage: "play.fill", prominent: true, action: model.playNextNow)
        labelled(upNext.atCredits ? String(localized: "Watch credits") : String(localized: "Hide"), systemImage: "xmark", action: model.hideUpNext)
    }

    /// A control filling its tile, all of which takes the tap.
    private func tile(_ name: String, action: @escaping () -> Void, @ViewBuilder label: () -> some View) -> some View {
        Button(action: action) {
            label()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .contentShape(RoundedRectangle(cornerRadius: PlayerGlyph.tileRadius, style: .continuous))
        }
        .buttonStyle(TileStyle())
        .accessibilityLabel(Text(name))
    }

    /// A tile with its glyph and a word for what it does, white for the main choice ([prominent]).
    private func labelled(_ title: String, systemImage: String, prominent: Bool = false, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            HStack(spacing: KivoTokens.spacing * 2.5) {
                Image(systemName: systemImage)
                Text(title)
                    .font(.system(size: 15, weight: .medium))
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .contentShape(RoundedRectangle(cornerRadius: PlayerGlyph.tileRadius, style: .continuous))
        }
        .buttonStyle(TileStyle(prominent: prominent))
    }
}

/// A tile that lightens while pressed; white, for the main choice, if [prominent].
private struct TileStyle: ButtonStyle {
    var prominent = false

    func makeBody(configuration: Configuration) -> some View {
        if prominent {
            configuration.label
                .foregroundStyle(.black)
                .background(
                    .white.opacity(configuration.isPressed ? 0.8 : 1),
                    in: RoundedRectangle(cornerRadius: PlayerGlyph.tileRadius, style: .continuous)
                )
        } else {
            configuration.label
                .playerTile(pressed: configuration.isPressed)
        }
    }
}

/// The glass bar of controls: back 10, play or pause and forward 30, the
/// scrub bar, and Fill ([fill]) and Audio & Subtitles and Playback
/// settings ([panels]).
struct ControlBar: View {
    let model: PlayerModel
    var fill = false
    var panels = true

    var body: some View {
        HStack(spacing: KivoTokens.spacing * 2) {
            Transport(model: model)
            ScrubBar(model: model)
            if fill || panels {
                HStack(spacing: 0) {
                    if fill {
                        Button(action: model.toggleFill) {
                            Image(systemName: model.fillGlyph)
                        }
                        .accessibilityLabel(model.fill ? Text("Fit to Screen") : Text("Fill Screen"))
                        .accessibilityIdentifier("fill")
                    }
                    if panels { PanelButtons(model: model) }
                }
            }
        }
        .padding(.horizontal, KivoTokens.spacing * 2.5)
        .frame(minHeight: 60)
        .playerGlass(in: Capsule(), interactive: false)
        .buttonStyle(PlayerIconStyle())
        .foregroundStyle(.white)
    }
}

/// Back 10, play or pause, and forward 30; a spinner takes play's place
/// while the video loads or buffers.
private struct Transport: View {
    let model: PlayerModel

    var body: some View {
        let paused = model.state?.paused != false
        let side: CGFloat = 44
        HStack(spacing: 0) {
            Button { model.skip(by: -Markers.shared.BACK_MS) } label: {
                Image(systemName: "gobackward.10")
                    .font(PlayerGlyph.transport)
                    .frame(width: side, height: side)
            }
            .accessibilityLabel(Text("Back 10 seconds"))
            Group {
                if model.waiting {
                    ProgressView()
                        .tint(.white)
                } else {
                    Button(action: model.togglePlay) {
                        Image(systemName: paused ? "play.fill" : "pause.fill")
                            .font(PlayerGlyph.transport)
                            .contentTransition(.symbolEffect(.replace))
                            .frame(width: side, height: side)
                    }
                    .accessibilityLabel(paused ? Text("Play") : Text("Pause"))
                    .accessibilityIdentifier(paused ? "Play" : "Pause")
                }
            }
            .frame(width: side, height: side)
            Button { model.skip(by: Markers.shared.FORWARD_MS) } label: {
                Image(systemName: "goforward.30")
                    .font(PlayerGlyph.transport)
                    .frame(width: side, height: side)
            }
            .accessibilityLabel(Text("Forward 30 seconds"))
        }
        .sensoryFeedback(.impact(weight: .light), trigger: paused) { _, _ in model.controlsVisible }
    }
}

private extension PlayerModel {
    /// Loading, buffering or seeking, while a spinner takes play's place.
    var waiting: Bool {
        switch state?.status {
        case .loading, .buffering, .seeking: true
        default: false
        }
    }

    /// Fill's glyph: to fill the screen, or back to fit once it does.
    var fillGlyph: String {
        fill ? "arrow.down.right.and.arrow.up.left" : "arrow.up.left.and.arrow.down.right"
    }

    func toggleFill() {
        input()
        fill.toggle()
    }
}

/// Audio & Subtitles and Playback settings, each opening its panel.
private struct PanelButtons: View {
    let model: PlayerModel

    var body: some View {
        HStack(spacing: 0) {
            Button {
                model.sheet = .tracks
            } label: {
                Image(systemName: "captions.bubble")
            }
            .accessibilityLabel(Text("Audio & Subtitles"))
            .accessibilityIdentifier("tracks")
            .modifier(PlayerPanel(model: model, sheet: .tracks))
            Button {
                model.sheet = .settings
            } label: {
                Image(systemName: "gearshape")
            }
            .accessibilityLabel(Text("Playback Settings"))
            .accessibilityIdentifier("settings")
            .modifier(PlayerPanel(model: model, sheet: .settings))
        }
    }
}

/// The title, marked when it plays from a download, with an episode's
/// code and name under it.
struct PlayerTitle: View {
    let model: PlayerModel

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: KivoTokens.spacing) {
                Text(model.title)
                if model.offline {
                    Image(systemName: "arrow.down.circle.fill")
                        .font(.footnote)
                        .foregroundStyle(.white.opacity(0.75))
                        .accessibilityLabel(Text("Downloaded"))
                }
            }
            .font(.headline)
            // The end time moves on while paused too.
            TimelineView(.everyMinute) { context in
                let line = [model.subtitleLine, model.endsAt(from: context.date)].compactMap { $0 }.joined(separator: " · ")
                if !line.isEmpty {
                    Text(line)
                        .font(.footnote)
                        .foregroundStyle(.white.opacity(0.75))
                }
            }
        }
        .lineLimit(1)
        .foregroundStyle(.white)
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(.isHeader)
    }
}

/// The player's top row on a phone, in glass: Close, the title with an
/// episode's code and name under it unless the controls carry it
/// ([titled]), no wider than [titleWidth], which keeps it short of an
/// unfolded phone's fold; and Cast, AirPlay and Picture in Picture unless
/// a ``RouteRail`` has them ([routes]).
struct PlayerTopRow: View {
    let model: PlayerModel
    var titled = true
    var routes = true
    var titleWidth: CGFloat = 320
    let close: () -> Void

    var body: some View {
        HStack(spacing: KivoTokens.spacing * 3) {
            Button(action: close) {
                Image(systemName: "xmark")
                    .frame(width: 44, height: 44)
                    .playerGlass(in: Circle())
            }
            .accessibilityLabel(Text("Close"))
            .accessibilityIdentifier("Close")
            if titled {
                PlayerTitle(model: model)
                    .frame(maxWidth: titleWidth, alignment: .leading)
            }
            Spacer(minLength: 0)
            if routes {
                HStack(spacing: 0) {
                    RouteButtons(model: model)
                }
                .padding(.horizontal, KivoTokens.spacing)
                .playerGlass(in: Capsule(), interactive: false)
            }
        }
        .font(PlayerGlyph.control)
        .buttonStyle(PlayerIconStyle())
        .foregroundStyle(.white)
    }
}

/// Cast where there's a device to cast to, AirPlay, and Picture in Picture.
private struct RouteButtons: View {
    let model: PlayerModel

    var body: some View {
        // Only with a device to cast to, rather than an empty slot.
        if model.cast.buttonShown {
            CastButton(cast: model.cast)
                .frame(width: 44, height: 44)
        }
        AirPlayButton(active: model.airPlaying)
            .frame(width: 44, height: 44)
        Button(action: model.startPictureInPicture) {
            Image(systemName: "pip.enter")
        }
        .accessibilityLabel(Text("Picture in Picture"))
    }
}

/// Cast, AirPlay and Picture in Picture in a glass column at the foot of
/// the rail a screen keeps down one side, as an iPhone Duo's does: the
/// system's clock stands at its head, and nothing else in its foot. The
/// column's foot lines up with the controls' at [bottom] from the safe
/// area's.
struct RouteRail: View {
    /// The side the rail is on.
    enum Side {
        case leading
        case trailing

        /// The rail in a screen's [insets]: a wide inset on one side and none on the other.
        init?(_ insets: EdgeInsets) {
            if insets.leading >= Self.least, insets.trailing < 1 {
                self = .leading
            } else if insets.trailing >= Self.least, insets.leading < 1 {
                self = .trailing
            } else {
                return nil
            }
        }

        private static let least: CGFloat = 60
    }

    let model: PlayerModel
    let side: Side
    let insets: EdgeInsets
    let bottom: CGFloat

    var body: some View {
        VStack(spacing: 0) {
            RouteButtons(model: model)
        }
        .padding(.vertical, KivoTokens.spacing)
        .playerGlass(in: Capsule(), interactive: false)
        .font(PlayerGlyph.control)
        .buttonStyle(PlayerIconStyle())
        .foregroundStyle(.white)
        .keepsControlsUp(model)
        // Centred across the rail, its foot on the controls'.
        .frame(width: side == .leading ? insets.leading : insets.trailing)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: side == .leading ? .bottomLeading : .bottomTrailing)
        .padding(.bottom, insets.bottom + bottom)
        .ignoresSafeArea()
    }
}

/// The next episode under the video: its still, "Up next" or its
/// countdown, its code and title, and Play now, with Hide or Watch credits
/// while it shows, as at the credits. The buttons sit at the end of the
/// row, or under it where it's [narrow].
struct UpNextRow: View {
    let next: WorkDetail
    let upNext: UpNext
    let play: () -> Void
    let hide: () -> Void
    var narrow = false
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        let stillWatching = upNext.shows && upNext.phase == .stillWatching
        VStack(alignment: .leading, spacing: KivoTokens.spacing * 2.5) {
            HStack(spacing: KivoTokens.spacing * 3) {
                ArtworkImage(next.images.still ?? next.images.backdrop, title: next.title, pointWidth: 112)
                    .frame(width: 112, height: 63)
                    .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
                VStack(alignment: .leading, spacing: 2) {
                    Text(upNext.shows ? upNextLabel(upNext) : String(localized: "Up next"))
                        .font(.caption.weight(stillWatching ? .semibold : .regular))
                        .foregroundStyle(stillWatching ? palette.onPlayer : palette.onPlayerMuted)
                    Text(next.episode.map { "\(Display.app.episodeCode(episode: $0)) · \(next.title)" } ?? next.title)
                        .font(.subheadline.weight(.semibold))
                        .lineLimit(2)
                }
                Spacer(minLength: 0)
                if !narrow { buttons }
            }
            if narrow { buttons }
        }
        .foregroundStyle(.white)
        .padding(KivoTokens.spacing * 2.5)
        .playerGlass(in: RoundedRectangle(cornerRadius: 22, style: .continuous), interactive: false)
    }

    private var buttons: some View {
        HStack(spacing: KivoTokens.spacing * 2) {
            Button(String(localized: "Play now"), action: play)
                .buttonStyle(PlayerPillStyle(prominent: true))
            if upNext.shows {
                Button(upNext.atCredits ? String(localized: "Watch credits") : String(localized: "Hide"), action: hide)
                    .buttonStyle(PlayerPillStyle(prominent: false))
            }
        }
    }
}

/// What up next says while it shows: still watching, waiting, or its countdown.
func upNextLabel(_ upNext: UpNext) -> String {
    switch upNext.phase {
    case .stillWatching: String(localized: "Still watching?")
    case .waiting: String(localized: "Up next")
    default: String(localized: "Up next in \(upNext.secondsLeft)")
    }
}

/// The panel a bottom-bar button opens, Audio & Subtitles or Playback
/// settings: a popover from the button in a regular-width window, and a
/// sheet in a compact one, as on a phone; Audio & Subtitles' is wider, for
/// its two lists.
struct PlayerPanel: ViewModifier {
    let model: any PlayerChoices
    let sheet: PlayerSheet

    func body(content: Content) -> some View {
        content.popover(isPresented: shown) {
            Group {
                switch sheet {
                case .tracks:
                    NavigationStack { TracksPanel(model: model) }
                        // Wide enough for its two lists.
                        .presentationSizing(.page)
                case .settings: PlaybackSettingsSheet(model: model)
                }
            }
            // A popover's size; a sheet takes the screen's.
            .frame(idealWidth: sheet == .tracks ? TracksPanel.width : KivoTokens.Layout.playerPanelWidth, idealHeight: KivoTokens.Layout.chapterListHeight)
            .presentationCompactAdaptation(horizontal: .sheet, vertical: .sheet)
            .presentationDetents([.medium, .large])
            .presentationDragIndicator(.visible)
            .preferredColorScheme(.dark)
        }
    }

    private var shown: Binding<Bool> {
        Binding(
            get: { model.sheet == sheet },
            set: { if !$0, model.sheet == sheet { model.sheet = nil } }
        )
    }
}

/// The player's top bar on an iPad, in the system's toolbar so that it
/// keeps clear of the window's controls, and in its glass: Close, then the
/// title with an episode's code and name under it, and Cast, AirPlay and
/// Picture in Picture.
struct PlayerTopBar: ToolbarContent {
    let model: PlayerModel
    /// How wide the title may grow.
    var titleWidth: CGFloat = 320
    let close: () -> Void

    var body: some ToolbarContent {
        ToolbarItem(placement: .topBarLeading) {
            Button(action: close) {
                Image(systemName: "xmark")
            }
            .accessibilityLabel(Text("Close"))
            .accessibilityIdentifier("Close")
        }
        if #available(iOS 26.0, *) {
            ToolbarItem(placement: .topBarLeading) { title }
                .sharedBackgroundVisibility(.hidden)
        } else {
            ToolbarItem(placement: .topBarLeading) { title }
        }
        ToolbarItemGroup(placement: .topBarTrailing) {
            CastButton(cast: model.cast)
            AirPlayButton(active: model.airPlaying)
                .frame(width: 44, height: 44)
            Button(action: model.startPictureInPicture) {
                Image(systemName: "pip.enter")
            }
            .accessibilityLabel(Text("Picture in Picture"))
        }
    }

    private var title: some View {
        PlayerTitle(model: model)
            // As wide as it reads, up to the width it may have: the bar would
            // otherwise squeeze it to a few letters.
            .frame(maxWidth: titleWidth, alignment: .leading)
            .fixedSize()
    }
}

/// The player's glyph sizes and tile shape, so its controls read as one set.
enum PlayerGlyph {
    /// Close, Cast, AirPlay, Picture in Picture, Fill, Audio & Subtitles and
    /// Playback settings.
    static let control = Font.system(size: 19, weight: .semibold)
    /// Back 10, play or pause and forward 30 in a bar.
    static let transport = Font.system(size: 22, weight: .semibold)
    /// Back 10, play or pause and forward 30 as tiles on a table: large
    /// and light, as the tiles are.
    static let transportTile = Font.system(size: 44, weight: .light)
    /// The other tiles on a table.
    static let controlTile = Font.system(size: 22, weight: .regular)
    /// Every tile's corners.
    static let tileRadius: CGFloat = 19
    /// How far a bar glyph sits inside its 44-point target.
    static let inset: CGFloat = 11
    /// From the safe area's sides to the rows of controls over the video.
    static let edge: CGFloat = 16
}

extension View {
    /// Controls that take a tap missing their buttons as use, keeping them
    /// up, rather than letting it through to the screen, which hides them.
    func keepsControlsUp(_ model: PlayerModel) -> some View {
        contentShape(Rectangle())
            .onTapGesture { model.input() }
    }

    /// A flat tile behind a control across a table, lighter while [pressed].
    func playerTile(pressed: Bool = false) -> some View {
        let shape = RoundedRectangle(cornerRadius: PlayerGlyph.tileRadius, style: .continuous)
        return background {
            shape.fill(.white.opacity(pressed ? 0.22 : 0.13))
            shape.strokeBorder(.white.opacity(0.08))
        }
    }

    /// Liquid Glass behind a player control, reacting to touch when
    /// [interactive]; a material before iOS 26. None unless [enabled].
    @ViewBuilder func playerGlass(in shape: some Shape, interactive: Bool = true, when enabled: Bool = true) -> some View {
        if !enabled {
            self
        } else if #available(iOS 26.0, *) {
            glassEffect(interactive ? .regular.interactive() : .regular, in: shape)
        } else {
            background(.ultraThinMaterial, in: shape)
        }
    }
}

/// A control's glyph with at least a 44-point square to tap, dimming while pressed.
struct PlayerIconStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(PlayerGlyph.control)
            .frame(minWidth: 44, minHeight: 44)
            .contentShape(Rectangle())
            .opacity(configuration.isPressed ? 0.5 : 1)
    }
}
