package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.WorkSummary

/** An episode's air date as `YYYY-MM-DD`, for the platform to format. */
fun EpisodeInfo.isoAirDate(): String? = airDate?.toString()

/** The season a series' page opens on, as Go to series asks: the season [id], else the one numbered [number]. */
data class SeasonRef(val id: String?, val number: Int)

internal object Seasons {
    /**
     * The season a series' page opens on, of its [seasons] as the server lists
     * them. A season [asked] for, by Go to series or by opening a season,
     * opens at once, found by its ID, else by its number; when it isn't among
     * them, the first season that isn't Specials does. With none asked, the
     * season holding [nextUp] does once the series is read or has failed to be
     * ([seriesRead]), else the first season that isn't Specials. Null while
     * there are no seasons, or while none was asked and the series isn't read.
     */
    fun openSeason(seasons: List<WorkSummary>, asked: SeasonRef?, nextUp: EpisodeInfo?, seriesRead: Boolean): WorkSummary? {
        if (seasons.isEmpty()) return null
        if (asked != null) {
            return asked.id?.let { id -> seasons.firstOrNull { it.id == id } }
                ?: seasons.firstOrNull { it.season?.number == asked.number }
                ?: seasons.firstRegular()
        }
        if (!seriesRead) return null
        return seasons.holding(nextUp) ?: seasons.firstRegular()
    }
}

private fun List<WorkSummary>.firstRegular(): WorkSummary = firstOrNull { it.season?.number != 0 } ?: first()

/** The season holding [episode]: by its season's ID, else by its season's number. */
private fun List<WorkSummary>.holding(episode: EpisodeInfo?): WorkSummary? {
    if (episode == null) return null
    return episode.seasonId?.let { id -> firstOrNull { it.id == id } } ?: firstOrNull { it.season?.number == episode.seasonNumber }
}
