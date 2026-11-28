import KivoCore

/// What Skip says for [marker]: Skip Intro, Skip Recap or Skip Credits.
func skipLabel(_ marker: SkipMarker) -> String {
    switch marker.kind {
    case .credits: String(localized: "Skip Credits")
    case .recap: String(localized: "Skip Recap")
    default: String(localized: "Skip Intro")
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

/// A stopped play in words: what failed, the server's reason, the
/// capability a file failed, and for the stream limit what plays where.
struct PlayerFailureWords {
    let title: String
    let sentence: String
    let limit: String?
    let plays: [PlaySummary]

    init(_ failure: PlayerFailure?, display: Display) {
        guard let failure else {
            title = String(localized: "Couldn't play this.")
            sentence = display.problem(code: "internal")
            limit = nil
            plays = []
            return
        }
        sentence = display.failure(failure: failure)
        switch onEnum(of: failure) {
        case .ended:
            title = String(localized: "Playback was stopped.")
            limit = nil
            plays = []
        case let .problem(problem):
            title = problem.knownCode == .streamLimit ? String(localized: "You're at your stream limit.") : String(localized: "Couldn't play this.")
            limit = problem.mismatch.map(StreamInfoRow.words)
            plays = problem.plays ?? []
        }
    }
}
