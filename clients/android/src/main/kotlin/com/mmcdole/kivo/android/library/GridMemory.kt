package com.mmcdole.kivo.android.library

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import com.mmcdole.kivo.android.ApiNamingMigration
import com.mmcdole.kivo.core.data.GridQuery
import java.net.URLDecoder
import java.net.URLEncoder
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

/** The values of the grid's sort and filters this app offers, as `listLibraryWorks` names them. */
object GridValues {
    const val TITLE = "title"
    const val ADDED_AT = "addedAt"
    const val LAST_EPISODE_ADDED_AT = "lastEpisodeAddedAt"
    const val RELEASE_DATE = "releaseDate"
    const val RATING = "rating"
    val sorts = listOf(TITLE, ADDED_AT, LAST_EPISODE_ADDED_AT, RELEASE_DATE, RATING)

    const val UNPLAYED = "unplayed"
    const val IN_PROGRESS = "inProgress"
    const val PLAYED = "played"
    val playedStates = listOf(UNPLAYED, IN_PROGRESS, PLAYED)

    /** Largest first, as the facets list them. */
    val resolutions = listOf("4k", "1080p", "720p", "sd")

    const val DOLBY_VISION = "dolbyVision"
    val dynamicRanges = listOf("hdr", DOLBY_VISION)
}

/** A library's section: its works, sorted and filtered, or its collections. */
enum class Section { Titles, Collections }

/** What a library's grid shows: its [section], and its works' sort and filters, kept while another section shows. */
data class GridChoice(val query: GridQuery, val section: Section = Section.Titles) {
    val libraryId: String get() = query.libraryId
}

/**
 * The section, sort and filters this device last used in each library,
 * kept in the app's DataStore, so a library opens as it was left.
 */
class SavedGrids(private val store: DataStore<Preferences>) {
    private fun key(libraryId: String) = stringPreferencesKey(PREFIX + libraryId)

    /** Every library's saved grid, by library ID, as each is saved. */
    val all: Flow<Map<String, GridChoice>> = store.data.map { preferences ->
        preferences.asMap().entries
            .mapNotNull { (key, value) ->
                val libraryId = key.name.takeIf { it.startsWith(PREFIX) }?.removePrefix(PREFIX)
                if (libraryId == null || value !is String) null else libraryId to decodeGrid(libraryId, value)
            }
            .toMap()
    }

    suspend fun save(choice: GridChoice) {
        store.edit { it[key(choice.libraryId)] = encodeGrid(choice) }
    }

    private companion object {
        const val PREFIX = "grid."
    }
}

/** A grid's section, sort and filters as `section=collections&sort=addedAt&resolution=4k,1080p`. */
fun encodeGrid(choice: GridChoice): String {
    val query = choice.query
    return listOfNotNull(
        "section=collections".takeIf { choice.section == Section.Collections },
        query.sort?.let { "sort=$it" },
        query.played?.let { "played=$it" },
        "favorite=1".takeIf { query.favorite },
        query.yearFrom?.let { "decade=$it" },
        query.genres?.takeIf { it.isNotEmpty() }?.let { "genre=" + encodeNames(it) },
        query.contentRatings?.takeIf { it.isNotEmpty() }?.let { "contentRating=" + encodeNames(it) },
        query.resolutions?.takeIf { it.isNotEmpty() }?.let { "resolution=" + it.joinToString(",") },
        query.dynamicRanges?.takeIf { it.isNotEmpty() }?.let { "dynamicRange=" + it.joinToString(",") },
    ).joinToString("&")
}

/** The choice [encodeGrid] wrote, leaving out what this version doesn't know. */
fun decodeGrid(libraryId: String, text: String): GridChoice {
    // Migration (api-naming): remove after every client has updated past this release.
    val values = ApiNamingMigration.savedGrid(
        text.split('&').mapNotNull { part -> part.split('=', limit = 2).takeIf { it.size == 2 }?.let { it[0] to it[1] } }.toMap(),
    )
    fun list(key: String) = values[key]?.split(',').orEmpty()
    val decade = values["decade"]?.toIntOrNull()
    val query = GridQuery(
        libraryId = libraryId,
        sort = values["sort"]?.takeIf { it in GridValues.sorts } ?: GridValues.TITLE,
        played = values["played"]?.takeIf { it in GridValues.playedStates },
        favorite = values["favorite"] == "1",
        yearFrom = decade,
        yearTo = decade?.plus(9),
        genres = decodeNames(values["genre"]),
        contentRatings = decodeNames(values["contentRating"]),
        resolutions = GridValues.resolutions.filter { it in list("resolution") }.ifEmpty { null },
        dynamicRanges = GridValues.dynamicRanges.filter { it in list("dynamicRange") }.ifEmpty { null },
    )
    return GridChoice(query, if (values["section"] == "collections") Section.Collections else Section.Titles)
}

/** Whether a filter narrows the works. */
val GridQuery.filtered: Boolean
    get() = played != null || favorite || yearFrom != null || !genres.isNullOrEmpty() || !contentRatings.isNullOrEmpty() ||
        !resolutions.isNullOrEmpty() || !dynamicRanges.isNullOrEmpty()

/** The query without its filters, keeping its sort. */
fun GridQuery.unfiltered(): GridQuery = copy(
    played = null,
    favorite = false,
    yearFrom = null,
    yearTo = null,
    genres = null,
    contentRatings = null,
    resolutions = null,
    dynamicRanges = null,
)

/** The decades a library's [years] fall in, newest first. */
fun decadesOf(years: List<Int>): List<Int> = years.map { it - it % 10 }.distinct().sortedDescending()

// Genres can hold the "&" and "," that separate a saved grid's parts, as "Action & Adventure" does.
private fun encodeNames(names: List<String>) = names.joinToString(",") { URLEncoder.encode(it, "UTF-8") }

private fun decodeNames(text: String?) = text?.split(',')?.map { URLDecoder.decode(it, "UTF-8") }?.ifEmpty { null }

/** The grid [libraryId] showed last, among the [saved] grids, else its works by title. */
fun gridChoice(saved: Map<String, GridChoice>, libraryId: String): GridChoice = saved[libraryId] ?: decodeGrid(libraryId, "")
