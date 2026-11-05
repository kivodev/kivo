package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.WorkSummary

/** An episode's air date as `YYYY-MM-DD`, for the platform to format. */
fun EpisodeInfo.isoAirDate(): String? = airDate?.toString()

object Seasons {
    /**
     * The season a series' page opens on, of its [seasons] as the server lists
     * them. A season asked for, by its summary's [seasonId] or as the season of
     * the [episode] Go to series came from, opens at once; when it isn't among
     * them, the first season that isn't Specials does. With none asked, the
     * season holding [nextUp] does once the series is read or has failed to be
     * ([seriesRead]), else the first season that isn't Specials. Null while
     * there are no seasons, or while none was asked and the series isn't read.
     */
    fun openSeason(
        seasons: List<WorkSummary>,
        seasonId: String?,
        episode: EpisodeInfo?,
        nextUp: EpisodeInfo?,
        seriesRead: Boolean,
    ): WorkSummary? {
        if (seasons.isEmpty()) return null
        val asked = seasonId != null || episode != null
        seasonId?.let { id -> seasons.firstOrNull { it.id == id } }?.let { return it }
        seasons.holding(episode)?.let { return it }
        if (!asked) {
            if (!seriesRead) return null
            seasons.holding(nextUp)?.let { return it }
        }
        return seasons.firstOrNull { it.season?.number != 0 } ?: seasons.first()
    }
}

/** The season holding [episode]: by its season's ID, else by its season's number. */
private fun List<WorkSummary>.holding(episode: EpisodeInfo?): WorkSummary? {
    if (episode == null) return null
    return episode.seasonId?.let { id -> firstOrNull { it.id == id } } ?: firstOrNull { it.season?.number == episode.seasonNumber }
}
