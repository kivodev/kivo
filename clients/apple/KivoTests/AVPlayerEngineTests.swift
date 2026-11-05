import AVFoundation
import KivoCore
import XCTest
@testable import Kivo

/// The core's engine on AVPlayer.
@MainActor
final class AVPlayerEngineTests: XCTestCase {
    func testAPlayLoadedOverAnotherReplacesItsItemWithoutEmptyingThePlayer() {
        let engine = Self.engine()
        var items: [AVPlayerItem?] = []
        let observation = engine.player.observe(\.currentItem, options: [.new]) { player, _ in
            items.append(player.currentItem)
        }
        engine.load(play: Self.play("p1", url: "plays/p1/media"), events: Events())
        engine.load(play: Self.play("p2", url: "plays/p2/media"), events: Events())
        observation.invalidate()

        XCTAssertEqual(items.count, 2)
        XCTAssertFalse(items.contains { $0 == nil }, "Picture in Picture closes on an empty player")

        engine.unload()
        XCTAssertNil(engine.player.currentItem)
    }

    /// A file with two audio tracks and a text track (Fixtures/two-audio.mp4,
    /// streams: 0 video, 1 English audio, 2 French audio, 3 English text):
    /// each plays by its stream index, and the subtitle shows and goes.
    func testAFilesAudioAndTextTracksAreSelectedByStreamIndex() async throws {
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: "two-audio", withExtension: "mp4"))
        let engine = Self.engine()
        engine.load(play: Self.play("p1", url: url.absoluteString), events: Events())
        engine.showSubtitle(show: TrackShowFile(index: 3))
        engine.selectAudio(index: 2)
        // The Simulator's media services can take a while to start.
        try await until("the file ready", seconds: 60) { engine.player.currentItem?.status == .readyToPlay }

        try await until("French audio and the text track") {
            let audio = await self.selected(.audible, in: engine)
            let text = await self.selected(.legible, in: engine)
            return audio?.extendedLanguageTag?.hasPrefix("fr") == true
                && text?.mediaType == .subtitle && text?.hasMediaCharacteristic(.containsOnlyForcedSubtitles) == false
        }

        engine.selectAudio(index: 1)
        engine.showSubtitle(show: TrackShowOff.shared)
        try await until("English audio and no subtitle") {
            let audio = await self.selected(.audible, in: engine)
            let text = await self.selected(.legible, in: engine)
            return audio?.extendedLanguageTag?.hasPrefix("en") == true && text == nil
        }
        engine.unload()
    }

    private func selected(_ characteristic: AVMediaCharacteristic, in engine: AVPlayerEngine) async -> AVMediaSelectionOption? {
        guard let item = engine.player.currentItem,
              let group = try? await item.asset.loadMediaSelectionGroup(for: characteristic)
        else { return nil }
        return item.currentMediaSelection.selectedMediaOption(in: group)
    }

    /// Waits up to [seconds] for [condition].
    private func until(_ what: String, seconds: Double = 10, _ condition: () async -> Bool) async throws {
        let deadline = Date().addingTimeInterval(seconds)
        while Date() < deadline {
            if await condition() { return }
            try await Task.sleep(for: .milliseconds(50))
        }
        XCTFail("timed out waiting for \(what)")
    }

    private static func engine() -> AVPlayerEngine {
        AVPlayerEngine(baseUrl: { "http://127.0.0.1:9" }, subtitleStyle: SubtitleStyle(size: .medium, background: .shadow))
    }

    private static func play(_ id: String, url: String) -> PlayStarted {
        PlayStarted(
            id: id,
            workId: "w1",
            versionId: "v1",
            startMs: 0,
            durationMs: 1_000,
            local: true,
            delivery: "file",
            url: url,
            tracks: [],
            parts: [PlayPart(durationMs: 1_000, url: nil)],
            chapters: [],
            bitrateBps: nil,
            limitBps: nil,
            previews: nil,
            markers: nil,
            quality: nil,
            dialogueBoost: nil
        )
    }
}

private final class Events: NSObject, PlayerEngineEvents {
    func onReady() {}
    func onPlay() {}
    func onPause() {}
    func onWaiting() {}
    func onPlaying() {}
    func onSeeked() {}
    func onTime() {}
    func onEnded() {}
    func onFailure(failure: PlayerError) {}
}
