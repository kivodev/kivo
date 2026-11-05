import KivoCore
import MediaPlayer

/// What the lock screen, Control Center, CarPlay, headphones, AirPods and a
/// car's steering wheel do to a book or a video playing: one table per kind,
/// set up on a command center by `setUp`.
///
/// Both show 10 seconds back and 30 forward, as the in-app players do; the
/// skip commands' intervals are what make the system draw skip buttons rather
/// than previous and next. A book's previous and next track skip by those
/// seconds too: a steering wheel's buttons, headphones' double and triple
/// press and AirPods send them, and in a book they mean a few seconds, not a
/// chapter. Chapters move in the app's Now Playing. A car's fast-forward
/// and rewind, which some send for its track arrows, skip a book the same
/// way, once a press, when the press ends; a video doesn't answer them.
///
/// A car's track buttons arrive as "next in context" and "previous in
/// context", which reach previous and next track only while Now Playing
/// shows no chapter number or count; with them, they're asked for a chapter
/// command that MPRemoteCommandCenter doesn't have, and go nowhere. So a
/// book's Now Playing leaves its chapter out of those fields.
enum RemoteCommands {
    enum Kind {
        case book
        case video
    }

    enum Action: Equatable {
        case play
        case pause
        case togglePlay
        case skipBack
        case skipForward
        /// To the event's position in what Now Playing's duration spans.
        case seek
    }

    /// The commands set up here; any one a kind doesn't answer is off.
    enum Command: CaseIterable {
        case play
        case pause
        case togglePlayPause
        case skipBackward
        case skipForward
        case previousTrack
        case nextTrack
        case changePlaybackPosition
        case seekBackward
        case seekForward

        func of(_ center: MPRemoteCommandCenter) -> MPRemoteCommand {
            switch self {
            case .play: center.playCommand
            case .pause: center.pauseCommand
            case .togglePlayPause: center.togglePlayPauseCommand
            case .skipBackward: center.skipBackwardCommand
            case .skipForward: center.skipForwardCommand
            case .previousTrack: center.previousTrackCommand
            case .nextTrack: center.nextTrackCommand
            case .changePlaybackPosition: center.changePlaybackPositionCommand
            case .seekBackward: center.seekBackwardCommand
            case .seekForward: center.seekForwardCommand
            }
        }
    }

    /// Back and forward, in seconds, as the skip buttons show them.
    static let backSeconds = Markers.shared.BACK_MS / 1000
    static let forwardSeconds = Markers.shared.FORWARD_MS / 1000

    static func actions(_ kind: Kind) -> [Command: Action] {
        let common: [Command: Action] = [
            .play: .play,
            .pause: .pause,
            .togglePlayPause: .togglePlay,
            .skipBackward: .skipBack,
            .skipForward: .skipForward,
            .changePlaybackPosition: .seek,
        ]
        switch kind {
        case .book:
            return common.merging([
                .previousTrack: .skipBack,
                .nextTrack: .skipForward,
                .seekBackward: .skipBack,
                .seekForward: .skipForward,
            ]) { $1 }
        case .video:
            return common
        }
    }

    /// Answers [kind]'s commands on [center] with [perform] and turns the
    /// others off. Returns the commands answered with their targets, to
    /// enable, disable and remove.
    static func setUp(
        _ center: MPRemoteCommandCenter,
        for kind: Kind,
        enabled: Bool = true,
        perform: @escaping (Action, MPRemoteCommandEvent) -> MPRemoteCommandHandlerStatus
    ) -> [(MPRemoteCommand, Any)] {
        center.skipBackwardCommand.preferredIntervals = [NSNumber(value: backSeconds)]
        center.skipForwardCommand.preferredIntervals = [NSNumber(value: forwardSeconds)]
        let actions = actions(kind)
        return Command.allCases.compactMap { command in
            let remote = command.of(center)
            guard let action = actions[command] else {
                remote.isEnabled = false
                return nil
            }
            remote.isEnabled = enabled
            return (remote, remote.addTarget { event in
                let seek = (event as? MPSeekCommandEvent)?.type
                return performs(seek) ? perform(action, event) : .success
            })
        }
    }

    /// Whether a command's event performs its action: a seek's only as the
    /// press ends, so a press skips once.
    static func performs(_ seek: MPSeekCommandEventType?) -> Bool {
        seek == nil || seek == .endSeeking
    }
}
