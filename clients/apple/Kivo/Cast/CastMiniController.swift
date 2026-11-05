import KivoCore
import KivoUI
import SwiftUI

/// The mini controller while casting, in the book mini player's place: the
/// work's still, its title over "Casting to Living Room TV", and Play or
/// Pause. When the cast can't go on, as when the TV can't reach the server,
/// the reason takes the device's line, with Retry. A tap opens the remote.
struct CastMiniController: View {
    let cast: CastModel
    let open: () -> Void
    var compact = false
    @Environment(\.kivoPalette) private var palette

    var body: some View {
        HStack(spacing: KivoTokens.spacing * 3) {
            Button(action: open) {
                HStack(spacing: KivoTokens.spacing * 3) {
                    CastArtwork(work: cast.work, pointWidth: 64)
                        .frame(width: compact ? 40 : 56, height: compact ? 24 : 32)
                        .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.sm / 2))
                    VStack(alignment: .leading, spacing: 1) {
                        Text(cast.title)
                            .font(.subheadline.weight(.semibold))
                            .lineLimit(1)
                        if !compact {
                            if let problem = cast.problem {
                                Text(problem)
                                    .font(.caption)
                                    .foregroundStyle(palette.danger)
                                    .lineLimit(1)
                            } else {
                                Label {
                                    Text(cast.casting)
                                } icon: {
                                    CastGlyph(connection: .connected).frame(width: 11, height: 11)
                                }
                                .font(.caption)
                                .foregroundStyle(.secondary)
                                .lineLimit(1)
                            }
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityHint(Text("Opens the remote"))
            .accessibilityIdentifier("castMiniOpen")
            Group {
                if cast.problem != nil {
                    Button(action: cast.retry) {
                        Image(systemName: "arrow.clockwise").font(.system(size: compact ? 16 : 19, weight: .semibold))
                    }
                    .accessibilityLabel(Text("Retry"))
                } else if cast.waiting {
                    ProgressView()
                } else {
                    Button(action: cast.togglePlay) {
                        Image(systemName: cast.playing ? "pause.fill" : "play.fill")
                            .font(.system(size: compact ? 17 : 22, weight: .semibold))
                            .contentTransition(.symbolEffect(.replace))
                    }
                    .accessibilityLabel(cast.playing ? Text("Pause") : Text("Play"))
                    .accessibilityIdentifier("castMiniPlay")
                }
            }
            .buttonStyle(.plain)
            .frame(width: 36, height: 36)
            .contentShape(Rectangle())
        }
        .padding(.leading, compact ? KivoTokens.spacing * 2 : KivoTokens.spacing * 3)
        .padding(.trailing, KivoTokens.spacing * 2)
        .contentShape(Rectangle())
        .simultaneousGesture(DragGesture(minimumDistance: 12).onEnded { value in
            if value.translation.height < -24 { open() }
        })
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("castMiniController")
    }
}

/// The work on the TV as a still: an episode's own, else the backdrop.
struct CastArtwork: View {
    let work: WorkDetail?
    let pointWidth: CGFloat

    var body: some View {
        ArtworkImage(work.flatMap { $0.images.still ?? $0.images.backdrop }, title: work?.title ?? "", pointWidth: pointWidth)
    }
}
