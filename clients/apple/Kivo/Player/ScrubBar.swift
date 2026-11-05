import KivoCore
import KivoUI
import SwiftUI

/// What the scrub bar shows and moves: the player's play, or a cast's on the TV.
@MainActor
protocol Scrubbable: AnyObject {
    var scrubDurationMs: Int64 { get }
    var scrubPositionMs: Int64 { get }
    var chapters: [Chapter] { get }
    /// The play's preview frames; nil shows the time and chapter alone.
    var previews: ScrubPreviews? { get }
    var scrubbing: Bool { get set }
    func seek(to positionMs: Int64)
    func skip(by deltaMs: Int64)
}

/// The scrub bar between the time played and the time left.
/// Touching it jumps the thumb under the finger; dragging moves it, more
/// finely the further the finger slides up or down away from the bar (half,
/// quarter and fine speed), with the time, the chapter and the preview
/// frame in a bubble above; it seeks on release. Chapters are ticks, and
/// crossing one ticks.
struct ScrubBar: View {
    let model: any Scrubbable
    @Environment(\.kivoPalette) private var palette
    @State private var target: Double?
    @State private var lastX: CGFloat = 0
    @State private var rate: Double = 1

    var body: some View {
        let duration = Double(model.scrubDurationMs)
        let shown = target ?? Double(model.scrubPositionMs)
        HStack(spacing: KivoTokens.spacing * 3) {
            Text(Display.app.clock(ms: Int64(shown)))
                .frame(minWidth: 48, alignment: .leading)
            GeometryReader { geometry in
                track(width: geometry.size.width, duration: duration, shown: shown)
            }
            .frame(height: 44)
            Text("-" + Display.app.clock(ms: Int64(max(0, duration - shown))))
                .frame(minWidth: 52, alignment: .trailing)
        }
        .font(.caption.monospacedDigit().weight(.medium))
        .foregroundStyle(.white.opacity(0.9))
        .sensoryFeedback(.selection, trigger: chapterIndex)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text("Playback position"))
        .accessibilityValue(Text("\(Display.app.clock(ms: Int64(shown))) of \(Display.app.clock(ms: Int64(duration)))"))
        .accessibilityIdentifier("scrub bar")
        .accessibilityAdjustableAction { direction in
            model.skip(by: direction == .increment ? Markers.shared.FORWARD_MS : -Markers.shared.BACK_MS)
        }
    }

    /// The bar, its chapter ticks and thumb, and the bubble while scrubbing.
    private func track(width: CGFloat, duration: Double, shown: Double) -> some View {
        let fraction: Double = duration > 0 ? min(1, max(0, shown / duration)) : 0
        let scrubbing = target != nil
        let height: CGFloat = scrubbing ? 8 : 4
        let thumb: CGFloat = scrubbing ? 22 : 14
        let chapters = Array(model.chapters.dropFirst())
        return ZStack(alignment: .leading) {
            Capsule()
                .fill(.white.opacity(0.3))
                .frame(height: height)
            Capsule()
                .fill(palette.progress)
                .frame(width: max(0, width * fraction), height: height)
            ForEach(chapters, id: \.startMs) { chapter in
                Rectangle()
                    .fill(.black.opacity(0.6))
                    .frame(width: 2, height: height)
                    .offset(x: width * Double(chapter.startMs) / max(duration, 1) - 1)
            }
            Circle()
                .fill(.white)
                .frame(width: thumb, height: thumb)
                .shadow(color: .black.opacity(0.3), radius: 2)
                .offset(x: width * fraction - thumb / 2)
        }
        .frame(width: width, height: 44)
        .contentShape(Rectangle())
        .gesture(
            DragGesture(minimumDistance: 0)
                .onChanged { value in drag(value, width: width, duration: duration) }
                .onEnded { _ in release() }
        )
        .overlay(alignment: .bottomLeading) {
            if let target {
                bubble(at: target, duration: duration)
                    .fixedSize()
                    // Centred over the thumb, kept from running off either end.
                    .alignmentGuide(.leading) { size in
                        let half: CGFloat = size.width / 2
                        let thumbX: CGFloat = width * CGFloat(fraction)
                        let centre: CGFloat = min(max(thumbX, half - 48), width - half + 48)
                        return half - centre
                    }
                    .offset(y: -52)
                    .allowsHitTesting(false)
            }
        }
        .animation(.easeOut(duration: 0.15), value: scrubbing)
        // Turning the phone mid-drag swaps the bar for another layout's,
        // and the drag ends without a release: it's let go of unseeked.
        .onDisappear {
            if target != nil { model.scrubbing = false }
        }
    }

    /// The chapter under the thumb while scrubbing, for the tick on crossing one.
    private var chapterIndex: Int? {
        let chapters = model.chapters
        guard let target, chapters.count > 1 else { return nil }
        return chapters.lastIndex { Double($0.startMs) <= target }
    }

    private func drag(_ value: DragGesture.Value, width: CGFloat, duration: Double) {
        guard width > 0, duration > 0 else { return }
        if target == nil {
            model.scrubbing = true
            target = min(duration, max(0, Double(value.location.x / width) * duration))
            lastX = value.location.x
            rate = 1
            return
        }
        let away = abs(value.location.y - 22)
        rate = away < 60 ? 1 : away < 120 ? 0.5 : away < 180 ? 0.25 : 0.1
        let moved = Double((value.location.x - lastX) / width) * duration * rate
        target = min(duration, max(0, (target ?? 0) + moved))
        lastX = value.location.x
    }

    private func release() {
        if let target { model.seek(to: Int64(target)) }
        target = nil
        model.scrubbing = false
    }

    /// The bubble over the thumb: the preview frame, the time, the chapter and the scrubbing speed.
    private func bubble(at target: Double, duration: Double) -> some View {
        VStack(spacing: KivoTokens.spacing) {
            if let previews = model.previews {
                PreviewFrame(previews: previews, positionMs: Int64(target))
            }
            Text(Display.app.clock(ms: Int64(target)))
                .font(.subheadline.monospacedDigit().weight(.semibold))
            if let chapter = Markers.shared.chapterAt(chapters: model.chapters, positionMs: Int64(target))?.title {
                Text(chapter).font(.caption).lineLimit(1).frame(maxWidth: 200)
            }
            if rate < 1 {
                Text(rateLabel).font(.caption2.weight(.semibold)).foregroundStyle(.white.opacity(0.75))
            }
        }
        .foregroundStyle(.white)
        .padding(KivoTokens.spacing * 1.5)
        .background(.black.opacity(0.65), in: RoundedRectangle(cornerRadius: KivoTokens.Radius.md))
    }

    private var rateLabel: String {
        switch rate {
        case 0.5: String(localized: "Half-Speed Scrubbing")
        case 0.25: String(localized: "Quarter-Speed Scrubbing")
        default: String(localized: "Fine Scrubbing")
        }
    }
}

/// A scrub preview: the frame's place in its sheet, cut out.
private struct PreviewFrame: View {
    let previews: ScrubPreviews
    let positionMs: Int64

    var body: some View {
        if let frame = SpriteFrame.companion.atOrNull(manifest: previews.manifest, manifestUrl: previews.url, positionMs: positionMs),
           let url = URL(string: frame.sheetUrl) {
            let manifest = previews.manifest
            // At most 180 points wide, so the bubble stays clear of the controls.
            let scale = min(1, 180 / max(1, CGFloat(frame.width)))
            AsyncSheet(url: url)
                .frame(width: CGFloat(manifest.columns * manifest.width) * scale, height: CGFloat(manifest.rows * manifest.height) * scale, alignment: .topLeading)
                .offset(x: -CGFloat(frame.x) * scale, y: -CGFloat(frame.y) * scale)
                .frame(width: CGFloat(frame.width) * scale, height: CGFloat(frame.height) * scale, alignment: .topLeading)
                .clipped()
                .clipShape(RoundedRectangle(cornerRadius: KivoTokens.Radius.sm))
        }
    }
}

/// A preview sheet, which the play's URL authorizes.
private struct AsyncSheet: View {
    let url: URL

    var body: some View {
        AsyncImage(url: url) { image in
            image.resizable()
        } placeholder: {
            Color.black
        }
    }
}
