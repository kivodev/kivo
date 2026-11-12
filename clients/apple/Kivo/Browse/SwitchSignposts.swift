import os

/// Points of interest for opening a page from the Libraries tab's list,
/// for Instruments and the performance tests: intervals from the
/// choice to its title showing, and to its first cards.
@MainActor
enum SwitchSignposts {
    static let subsystem = "com.kivo.player"
    private static let signposter = OSSignposter(subsystem: subsystem, category: .pointsOfInterest)
    private static var title: OSSignpostIntervalState?
    private static var cards: OSSignpostIntervalState?

    /// A page was opened from the list; a switch still under way ends unfinished.
    static func chose() {
        end(&title, "Switch title")
        end(&cards, "Switch cards")
        title = signposter.beginInterval("Switch title")
        cards = signposter.beginInterval("Switch cards")
    }

    static func titleShown() { end(&title, "Switch title") }

    static func cardsShown() { end(&cards, "Switch cards") }

    private static func end(_ state: inout OSSignpostIntervalState?, _ name: StaticString) {
        guard let started = state else { return }
        signposter.endInterval(name, started)
        state = nil
    }
}
