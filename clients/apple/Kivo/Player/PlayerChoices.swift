import KivoCore
import SwiftUI

/// What Audio & Subtitles, Playback Settings and Stream info show and
/// change: the player's own play, or a cast's on the TV.
@MainActor
protocol PlayerChoices: AnyObject {
    var display: Display { get }
    /// The sheet open: Audio & Subtitles, or Playback settings.
    var sheet: PlayerSheet? { get set }
    var work: WorkDetail? { get }
    /// The version playing.
    var version: Version? { get }
    /// Playing a download from its files: no quality, version or other track to choose.
    var offline: Bool { get }
    var audioTrackId: String? { get }
    var dialogueBoost: DialogueBoost? { get }
    /// The subtitle track showing, or off.
    var subtitle: String { get }
    var qualities: [QualityOption] { get }
    /// The quality playing.
    var quality: Quality? { get }
    var speed: Float { get }
    /// What Stream info says of the play; nil before it starts.
    var streamInfo: StreamInfo? { get }
    /// Whether Subtitles offers "Search subtitles…": a video the server
    /// plays, even with no subtitle track, never a download from its files.
    var searchesSubtitles: Bool { get }

    /// Whether [track] can be chosen: a download has only the tracks it kept.
    func offers(_ track: Track) -> Bool
    func chooseAudio(_ trackId: String)
    func chooseSubtitle(_ trackId: String)
    func boostDialogue(_ on: Bool)
    func chooseQuality(_ option: QualityOption)
    func chooseVersion(_ versionId: String)
    func setSpeed(_ speed: Float)
    /// The subtitle search's results for the version playing, best first.
    func searchSubtitles() async throws -> [SubtitleResult]
    /// Downloads a result to the server, adds its track to the version and plays on with it.
    func downloadSubtitle(_ result: SubtitleResult) async throws
}
