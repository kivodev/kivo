import KivoCore
import KivoUI
import SwiftUI

/// The Cast menu, which the lit Cast button opens: the Cast device by
/// name, what plays on it with Play or Pause, the TV's volume, and Stop
/// casting, which carries on here from the TV's position, or Disconnect,
/// which leaves the TV playing.
struct CastMenu: View {
    let cast: CastModel
    /// Carries on here from where the TV was.
    let stop: () -> Void
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            List {
                Section {
                    if cast.active {
                        nowPlaying
                    }
                    HStack(spacing: KivoTokens.spacing * 3) {
                        Image(systemName: "speaker.fill").foregroundStyle(.secondary)
                        Slider(value: Binding(get: { Double(cast.volume) }, set: { cast.setVolume(Float($0)) }), in: 0 ... 1)
                            .accessibilityLabel(Text("TV volume"))
                        Image(systemName: "speaker.wave.3.fill").foregroundStyle(.secondary)
                    }
                    .font(.footnote)
                }
                Section {
                    if cast.active {
                        row(String(localized: "Stop casting"), detail: continueHere, symbol: UIDevice.current.userInterfaceIdiom == .pad ? "ipad" : "iphone", action: stop)
                            .accessibilityIdentifier("stopCasting")
                        row(String(localized: "Disconnect"), detail: String(localized: "Keep playing on the TV"), symbol: "tv", action: cast.disconnect)
                            .accessibilityIdentifier("disconnect")
                    } else {
                        row(String(localized: "Stop casting"), detail: nil, symbol: "xmark.circle", action: cast.disconnect)
                            .accessibilityIdentifier("stopCasting")
                    }
                }
            }
            .listSectionSpacing(.compact)
            .contentMargins(.top, KivoTokens.spacing * 2, for: .scrollContent)
            .navigationTitle(cast.device ?? "")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button { dismiss() } label: { Image(systemName: "xmark") }
                        .accessibilityLabel(Text("Close"))
                        .accessibilityIdentifier("castMenuClose")
                }
            }
        }
        .presentationDetents([.medium])
        .presentationDragIndicator(.visible)
    }

    private var continueHere: String {
        UIDevice.current.userInterfaceIdiom == .pad ? String(localized: "Continue on this iPad") : String(localized: "Continue on this iPhone")
    }

    /// The work on the TV, where it is and whether it plays, with Play or Pause.
    private var nowPlaying: some View {
        HStack(spacing: KivoTokens.spacing * 3) {
            CastArtwork(work: cast.work, pointWidth: 64)
                .frame(width: 56, height: 32)
                .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.sm / 2))
            VStack(alignment: .leading, spacing: 1) {
                Text(cast.title).font(.body.weight(.semibold)).lineLimit(1)
                Text(state).font(.footnote).foregroundStyle(.secondary).lineLimit(1)
            }
            Spacer(minLength: 0)
            if cast.waiting {
                ProgressView()
            } else {
                Button(action: cast.togglePlay) {
                    Image(systemName: cast.playing ? "pause.fill" : "play.fill")
                        .font(.title3)
                        .frame(width: 44, height: 44)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel(cast.playing ? Text("Pause") : Text("Play"))
            }
        }
    }

    /// "23:38 · Playing", "Paused" or "Loading".
    private var state: String {
        let time = Display.app.clock(ms: cast.positionMs)
        if cast.waiting { return String(localized: "Loading") }
        return cast.playing ? String(localized: "\(time) · Playing") : String(localized: "\(time) · Paused")
    }

    private func row(_ title: String, detail: String?, symbol: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Label {
                VStack(alignment: .leading, spacing: 1) {
                    Text(title).foregroundStyle(.primary)
                    if let detail {
                        Text(detail).font(.footnote).foregroundStyle(.secondary)
                    }
                }
            } icon: {
                Image(systemName: symbol)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }
}
