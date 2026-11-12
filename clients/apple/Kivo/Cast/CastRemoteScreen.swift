import KivoCore
import KivoUI
import SwiftUI

/// The remote while casting, upright on a phone: the work's still with
/// "Casting to Living Room TV", the title and its line, the player's scrub
/// bar, Back 10, Play or Pause and Forward 30, up next when the TV shows it,
/// and Audio & Subtitles and Playback Settings, which act on the TV. The
/// TV's volume is the phone's volume buttons, and the Cast menu's. A
/// spinner takes Play's place while the TV loads or buffers. When the cast
/// can't go on, the reason and Try again take the scrub bar's and the
/// transport's place. With up next showing, the still is
/// smaller and the rows closer, so everything stays on the screen. The
/// chevron closes it to the mini controller, and the cast plays on.
struct CastRemoteScreen: View {
    let cast: CastModel
    let close: () -> Void
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        NavigationStack {
            GeometryReader { geometry in
                ScrollView {
                    content
                        .padding(.horizontal, 24)
                        .padding(.bottom, KivoTokens.spacing * 2)
                        .frame(maxWidth: 520, minHeight: geometry.size.height)
                        .frame(maxWidth: .infinity)
                }
                .scrollBounceBehavior(.basedOnSize)
            }
            .background { RemoteBackground(work: cast.work) }
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button(action: close) {
                        Image(systemName: "chevron.down")
                    }
                    .accessibilityLabel(Text("Close"))
                    .accessibilityIdentifier("Close")
                }
                ToolbarItem(placement: .topBarTrailing) {
                    CastButton(cast: cast)
                }
            }
            .toolbarBackgroundVisibility(.hidden, for: .navigationBar)
            .navigationBarTitleDisplayMode(.inline)
        }
        .tint(palette.onPlayer)
        .preferredColorScheme(.dark)
        .onAppear { Orientation.request(.portrait) }
        .onDisappear { Orientation.follow() }
    }

    /// Top to bottom, the room left over split above the scrub bar and above the panels.
    private var content: some View {
        let upNext = cast.status?.upNext.shows == true ? cast.next : nil
        let gap = KivoTokens.spacing * (upNext == nil ? 1 : 0.5)
        return VStack(spacing: 0) {
            CastArtwork(work: cast.work, pointWidth: 520)
                .aspectRatio(16 / 9, contentMode: .fit)
                .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
                .shadow(color: .black.opacity(0.35), radius: 20, y: 10)
                .frame(maxHeight: upNext == nil ? nil : 132)
                .padding(.top, gap * 2)
            Label {
                Text(cast.casting)
            } icon: {
                CastGlyph(connection: .connected).frame(width: 15, height: 15)
            }
            .font(.subheadline.weight(.medium))
            .foregroundStyle(palette.accent)
            .padding(.top, gap * 5)
            titles
                .padding(.top, gap * 6)
            Spacer(minLength: gap * 6)
            if let problem = cast.problem {
                failure(problem)
            } else {
                ScrubBar(model: cast)
                    .disabled(cast.media == nil)
                    .opacity(cast.waiting ? 0.5 : 1)
                transport
                    .padding(.top, gap * 3)
            }
            if let upNext {
                CastUpNext(cast: cast, next: upNext)
                    .padding(.top, gap * 6)
            }
            Spacer(minLength: gap * 6)
            panels
        }
    }

    /// Why the cast can't go on, with Try again, as the player says why it stopped.
    private func failure(_ problem: String) -> some View {
        VStack(spacing: KivoTokens.spacing * 3) {
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 28, weight: .semibold))
                .foregroundStyle(palette.onPlayerMuted)
            Text(problem)
                .font(.callout)
                .foregroundStyle(palette.onPlayer)
                .multilineTextAlignment(.center)
            Button(String(localized: "Try again"), action: cast.retry)
                .buttonStyle(PlayerPillStyle(prominent: true))
                .accessibilityIdentifier("castRetry")
        }
        .frame(maxWidth: .infinity)
    }

    private var titles: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(cast.title)
                .font(.title2.weight(.bold))
                .lineLimit(2)
            if let line = cast.line {
                Text(line)
                    .font(.subheadline)
                    .foregroundStyle(palette.onPlayerMuted)
            }
        }
        .foregroundStyle(palette.onPlayer)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(.isHeader)
    }

    private var transport: some View {
        HStack(spacing: 0) {
            Button { cast.skip(by: -SKIP_BACK_MS) } label: {
                Image(systemName: "gobackward.10").font(.system(size: 30, weight: .medium))
            }
            .accessibilityLabel(Text("Back 10 seconds"))
            .frame(maxWidth: .infinity)
            Group {
                if cast.waiting {
                    ProgressView().controlSize(.large)
                } else {
                    Button(action: cast.togglePlay) {
                        Image(systemName: cast.playing ? "pause.fill" : "play.fill")
                            .font(.system(size: 46, weight: .medium))
                            .contentTransition(.symbolEffect(.replace))
                    }
                    .accessibilityLabel(cast.playing ? Text("Pause") : Text("Play"))
                    .accessibilityIdentifier(cast.playing ? "Pause" : "Play")
                }
            }
            .frame(width: 72, height: 72)
            Button { cast.skip(by: SKIP_FORWARD_MS) } label: {
                Image(systemName: "goforward.30").font(.system(size: 30, weight: .medium))
            }
            .accessibilityLabel(Text("Forward 30 seconds"))
            .frame(maxWidth: .infinity)
        }
        .buttonStyle(RemoteButtonStyle())
        .foregroundStyle(palette.onPlayer)
        .disabled(cast.media == nil)
        .sensoryFeedback(.impact(weight: .light), trigger: cast.playing)
    }

    private var panels: some View {
        HStack(spacing: 64) {
            Button { cast.sheet = .tracks } label: { Image(systemName: "captions.bubble") }
                .accessibilityLabel(Text("Audio & Subtitles"))
                .accessibilityIdentifier("tracks")
                .modifier(PlayerPanel(model: cast, sheet: .tracks))
            Button { cast.sheet = .settings } label: { Image(systemName: "gearshape") }
                .accessibilityLabel(Text("Playback Settings"))
                .accessibilityIdentifier("settings")
                .modifier(PlayerPanel(model: cast, sheet: .settings))
        }
        .font(.system(size: 21, weight: .medium))
        .buttonStyle(RemoteButtonStyle())
        .foregroundStyle(palette.onPlayer)
        .disabled(cast.status?.playing != true)
    }
}

/// Up next as the TV shows it: the next episode's still, the countdown and
/// its title, with Play now and Hide; after three episodes without input,
/// "Still watching?".
private struct CastUpNext: View {
    let cast: CastModel
    let next: WorkDetail
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        let upNext = cast.status?.upNext
        VStack(alignment: .leading, spacing: KivoTokens.spacing * 3) {
            HStack(spacing: KivoTokens.spacing * 3) {
                CastArtwork(work: next, pointWidth: 120)
                    .frame(width: 96, height: 54)
                    .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.sm))
                VStack(alignment: .leading, spacing: 2) {
                    if upNext?.phase == .stillWatching {
                        Text("Still watching?").font(.caption.weight(.semibold)).foregroundStyle(palette.onPlayer)
                    } else {
                        Text("Up next · \(upNext?.secondsLeft ?? 0)").font(.caption).foregroundStyle(palette.onPlayerMuted)
                    }
                    Text(next.episode.map { "\(Display.app.episodeCode(episode: $0)) · \(next.title)" } ?? next.title)
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(palette.onPlayer)
                        .lineLimit(2)
                }
                Spacer(minLength: 0)
            }
            HStack(spacing: KivoTokens.spacing * 2) {
                Button(action: cast.playNextNow) {
                    Label(String(localized: "Play now"), systemImage: "play.fill").frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .tint(palette.accent)
                .foregroundStyle(palette.onAccent)
                Button(action: cast.hideUpNext) {
                    Text("Hide").frame(maxWidth: .infinity)
                }
                .buttonStyle(.bordered)
                .tint(palette.onPlayer)
            }
            .controlSize(.large)
        }
        .padding(KivoTokens.spacing * 3)
        .background(palette.playerPanel, in: RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
    }
}

/// A soft blur of the work's backdrop over the player's dark.
private struct RemoteBackground: View {
    let work: WorkDetail?
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        ZStack {
            palette.player
            ArtworkImage(work.flatMap { $0.images.backdrop ?? $0.images.still }, title: "", pointWidth: 64)
                .scaleEffect(1.6)
                .blur(radius: 70)
                .saturation(1.4)
                .opacity(0.6)
            LinearGradient(colors: [.black.opacity(0.1), .black.opacity(0.6)], startPoint: .top, endPoint: .bottom)
        }
        .ignoresSafeArea()
        .accessibilityHidden(true)
    }
}

/// The remote's glyphs dim while pressed, as the system's do.
private struct RemoteButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var enabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .frame(minWidth: 44, minHeight: 44)
            .contentShape(Rectangle())
            .opacity(enabled ? (configuration.isPressed ? 0.45 : 1) : 0.35)
            .scaleEffect(configuration.isPressed ? 0.92 : 1)
            .animation(.easeOut(duration: 0.12), value: configuration.isPressed)
    }
}
