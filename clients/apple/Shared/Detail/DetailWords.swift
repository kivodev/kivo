import Foundation
import KivoCore
import KivoUI

/// A detail page's words: its details line, what Play says and what it
/// plays, and a series' seasons.
enum DetailWords {
    /// "1989 · 1h 42m · ★ 6.8 · PG · Comedy, Horror"; an episode's "S1 · E4 · Jan 28, 2018 · 44m".
    static func details(_ work: WorkSummary, genres: [String]?) -> [String] {
        var parts: [String] = []
        if let episode = work.episode {
            parts.append(Display.app.episodeCode(episode: episode))
            if let date = airDate(episode) {
                parts.append(date.formatted(date: .abbreviated, time: .omitted))
            }
        } else if let year = work.year {
            parts.append(String(year.intValue))
        }
        if let ms = work.durationMs?.int64Value { parts.append(Display.app.runtime(ms: ms)) }
        if let rating = work.communityRatingText { parts.append(rating) }
        if let content = work.contentRating { parts.append(content) }
        if work.episode == nil, let genres, !genres.isEmpty {
            parts.append(genres.prefix(2).joined(separator: ", "))
        }
        return parts
    }

    /// "2015 to 2022 · 6 seasons · ★ 8.5 · TV-MA".
    static func seriesDetails(_ series: WorkSummary) -> [String] {
        var parts: [String] = []
        if let year = series.year?.intValue {
            if let last = series.series?.lastAirYear?.intValue, last != year {
                parts.append(String(localized: "\(String(year)) to \(String(last))"))
            } else {
                parts.append(String(year))
            }
        }
        if let count = series.series?.seasonCount?.intValue {
            parts.append(count == 1 ? String(localized: "1 season") : String(localized: "\(count) seasons"))
        }
        if let rating = series.communityRatingText { parts.append(rating) }
        if let content = series.contentRating { parts.append(content) }
        return parts
    }

    /// The air date, sent as `YYYY-MM-DD`.
    static func airDate(_ episode: EpisodeInfo) -> Date? {
        guard let parts = episode.isoAirDate()?.split(separator: "-").compactMap({ Int($0) }), parts.count == 3 else { return nil }
        return Calendar.current.date(from: DateComponents(year: parts[0], month: parts[1], day: parts[2]))
    }

    /// "Resume 5:56", when the user has somewhere to resume [work] from on
    /// the version [versionId] of its page's [read] work. A version of
    /// another edition than the one the position was saved on starts from
    /// the beginning.
    static func resume(_ work: WorkSummary, read: WorkDetail?, versionId: String?) -> String? {
        guard let resumeMs = work.resumeMs else { return nil }
        let position = read.map { $0.resumeAt(versionId: versionId)?.int64Value } ?? resumeMs
        guard let position else { return nil }
        return String(localized: "Resume \(Display.app.clock(ms: position))")
    }

    /// "1 hr 36 min left · Ends at 9:41 PM", while [work] is in progress.
    static func timeLeft(_ work: WorkSummary, from now: Date) -> String? {
        guard let timeLeft = work.timeLeft, let resumeMs = work.resumeMs, let duration = work.durationMs?.int64Value else { return nil }
        return [timeLeft, Words.endsAt(kind: work.kind, remainingMs: duration - resumeMs, from: now)].compactMap { $0 }.joined(separator: " · ")
    }

    /// "Ends at 9:41 PM" for a movie or an episode, after its time left,
    /// "1 hr 36 min left · Ends at 9:41 PM", once it's started.
    static func endsLine(_ work: WorkSummary, from now: Date) -> String? {
        guard let duration = work.durationMs?.int64Value,
              let ends = Words.endsAt(kind: work.kind, remainingMs: duration - (work.resumeMs ?? 0), from: now)
        else { return nil }
        return [work.timeLeft, ends].compactMap { $0 }.joined(separator: " · ")
    }

    /// A series' Play: "Play S1 E1", or "Resume 12:03" for Next Up in progress.
    static func play(nextUp: WorkSummary) -> String {
        if nextUp.progress != nil, let position = nextUp.userState?.positionMs?.int64Value {
            return String(localized: "Resume \(Display.app.clock(ms: position))")
        }
        guard let episode = nextUp.episode else { return String(localized: "Play") }
        return String(localized: "Play S\(episode.seasonNumber) E\(episode.number)")
    }

    /// Under a series' Play: Next Up's title and runtime, or the time left in it.
    static func secondLine(nextUp: WorkSummary) -> String {
        let time = nextUp.timeLeft ?? nextUp.durationMs.map { Display.app.runtime(ms: $0.int64Value) }
        return [nextUp.title, time].compactMap { $0 }.joined(separator: " · ")
    }

    /// "S1", or Specials for season 0, or the season's title without a number.
    static func seasonLabel(_ season: WorkSummary) -> String {
        guard let number = season.season?.number else { return season.title }
        return number == 0 ? String(localized: "Specials") : String(localized: "S\(number)")
    }

    /// "3 unwatched", or Watched.
    static func seasonState(_ season: WorkSummary) -> String {
        guard let info = season.season else { return "" }
        return info.unplayedCount == 0 ? String(localized: "Watched") : String(localized: "\(info.unplayedCount) unwatched")
    }

    /// "10 episodes", when the season says how many.
    static func episodeCount(_ season: WorkSummary?) -> String? {
        guard let count = season?.season?.episodeCount?.intValue else { return nil }
        return count == 1 ? String(localized: "1 episode") : String(localized: "\(count) episodes")
    }
}
