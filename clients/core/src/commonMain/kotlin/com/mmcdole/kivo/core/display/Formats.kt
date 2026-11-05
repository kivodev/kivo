package com.mmcdole.kivo.core.display

import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.QualityRung
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.settings.Quality

// Numbers the API sends as integers (bitrates in bits per second, sizes in
// bytes, durations in milliseconds) in the locale's words. Every value is
// rounded half up in integers, a decimal shown only under 10.

/** A bitrate in megabits per second: "9.8 Mb/s", "20 Mb/s". */
fun Display.bitrate(bps: Long): String = table.bitrate.fill("n" to scaled(bps, 1_000_000))

/** A size: "58 GB", "5.1 GB", "700 MB", "2.4 MB", "300 KB". */
fun Display.size(bytes: Long): String {
    val (words, unit) = unitOf(bytes)
    return words.fill("n" to scaled(bytes, unit))
}

/** The words and the unit a size is said in. */
private fun Display.unitOf(bytes: Long): Pair<String, Long> {
    val sizes = table.sizes
    return when {
        bytes >= 1_000_000_000 -> sizes.gigabytes to 1_000_000_000
        bytes >= 1_000_000 -> sizes.megabytes to 1_000_000
        else -> sizes.kilobytes to 1_000
    }
}

/** A runtime in whole minutes, at least one: "2h 46m", "48m", "2h". */
fun Display.runtime(ms: Long): String =
    table.durations.run { inHours(ms, minutes, hours, hoursMinutes) }

/** Time left in whole minutes, at least one: "18 min left", "1 hr 34 min left", "2 hr left". */
fun Display.timeLeft(ms: Long): String =
    table.durations.run { inHours(ms, minutesLeft, hoursLeft, hoursMinutesLeft) }

/** Whole minutes, at least one, in minutes under an hour, else in hours and the minutes over. */
private fun Display.inHours(ms: Long, inMinutes: String, inWholeHours: String, inHoursMinutes: String): String {
    val minutes = minutesOf(ms)
    val hours = number(minutes / 60)
    val rest = number(minutes % 60)
    return when {
        minutes < 60 -> inMinutes.fill("minutes" to rest)
        minutes % 60 == 0L -> inWholeHours.fill("hours" to hours)
        else -> inHoursMinutes.fill("hours" to hours, "minutes" to rest)
    }
}

/** A position on a clock, at least zero: "1:12:03", "4:05". */
fun Display.clock(ms: Long): String {
    val total = ms.coerceAtLeast(0) / 1000
    val hours = total / 3600
    val minutes = total % 3600 / 60
    val seconds = (total % 60).toString().padStart(2, '0')
    return if (hours > 0) "$hours:${minutes.toString().padStart(2, '0')}:$seconds" else "$minutes:$seconds"
}

/** An episode's place: "S3 · E4", or "S1 · E1 to E2" for a file of several. */
fun Display.episodeCode(episode: EpisodeInfo): String {
    val place = arrayOf("season" to episode.seasonNumber.toString(), "episode" to episode.number.toString())
    val end = episode.endNumber?.takeIf { it != episode.number }
        ?: return table.episodeCode.one.fill(*place)
    return table.episodeCode.range.fill(*place, "end" to end.toString())
}

/** A rung of the quality ladder by its height and bitrate: "1080p · 8 Mb/s". */
fun Display.rung(rung: QualityRung): String =
    table.quality.rung.fill("height" to rung.height.toString(), "bitrate" to bitrate(rung.bitrateBps))

/**
 * A choice of the quality menu: "Auto"; "Original", with the file's
 * bitrate when [sourceBps] is given ("Original · 20 Mb/s"); its rung
 * ("1080p · 8 Mb/s"), or a limit's bitrate when the ladder has no rung for it.
 */
fun Display.quality(option: QualityOption, sourceBps: Long?): String {
    val words = table.quality
    return when (val quality = option.quality) {
        Quality.Auto -> words.auto
        Quality.Original -> sourceBps?.let { words.originalAt.fill("bitrate" to bitrate(it)) } ?: words.original
        is Quality.Limit -> option.rung?.let(::rung) ?: bitrate(quality.maxBitrateBps)
    }
}

/** A problem code in a sentence the user can act on; one this version doesn't know says something went wrong. */
fun Display.problem(code: String): String = table.problems[code] ?: table.unknownProblem

/** Why a request failed, in a sentence, by its code. */
fun Display.problem(problem: RequestProblem): String = problem(problem.code)

/** Whole minutes, at least one. */
private fun minutesOf(ms: Long): Long = maxOf(1, rounded(ms.coerceAtLeast(0), 60_000))

/** [value] divided by [unit], with a decimal under 10: "9.8", "20". */
private fun Display.scaled(value: Long, unit: Long): String {
    val v = value.coerceAtLeast(0)
    return if (v < 10 * unit) decimal(rounded(v, unit / 10)) else number(rounded(v, unit))
}

/** [n] over [d], rounded half up. */
private fun rounded(n: Long, d: Long): Long = (n + d / 2) / d

/** A count of tenths, its decimal left out when it's whole: "9.8", "10". */
private fun Display.decimal(tenths: Long): String =
    if (tenths % 10 == 0L) number(tenths / 10) else number(tenths / 10) + table.numbers.decimal + tenths % 10

/** A whole number, at least zero, grouped by thousands: "1,234". */
private fun Display.number(n: Long): String {
    val digits = n.toString()
    return buildString {
        digits.forEachIndexed { i, digit ->
            if (i > 0 && (digits.length - i) % 3 == 0) append(table.numbers.group)
            append(digit)
        }
    }
}

/** The words with each `{name}` replaced by its value. */
internal fun String.fill(vararg values: Pair<String, String>): String =
    values.fold(this) { words, (name, value) -> words.replace("{$name}", value) }
