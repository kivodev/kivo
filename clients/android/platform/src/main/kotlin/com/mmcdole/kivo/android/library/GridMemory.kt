package com.mmcdole.kivo.android.library

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import com.mmcdole.kivo.android.ApiNamingMigration
import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.Resolution
import com.mmcdole.kivo.core.data.DynamicRange
import com.mmcdole.kivo.core.data.GridQuery
import com.mmcdole.kivo.core.data.GridSort
import com.mmcdole.kivo.core.data.WorkPlayState
import java.net.URLDecoder
import java.net.URLEncoder
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

/**
 * The sorts a library of [kind] offers, in the menu's order: an
 * audiobooks library's by title, author, date added and release date;
 * any other's by title, date added, the last episode added for shows,
 * release date, rating, runtime and last played.
 */
fun sortsOf(kind: LibraryKind?): List<GridSort> = when (kind) {
    LibraryKind.Audiobooks -> listOf(GridSort.Title, GridSort.Author, GridSort.AddedAt, GridSort.ReleaseDate)
    LibraryKind.Shows -> listOf(
        GridSort.Title,
        GridSort.AddedAt,
        GridSort.LastEpisodeAddedAt,
        GridSort.ReleaseDate,
        GridSort.Rating,
        GridSort.Runtime,
        GridSort.LastPlayedAt,
    )
    else -> listOf(GridSort.Title, GridSort.AddedAt, GridSort.ReleaseDate, GridSort.Rating, GridSort.Runtime, GridSort.LastPlayedAt)
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

/** A grid's section, sort and filters as `section=collections&sort=addedAt&resolution=4k,1080p&narrator=p1`. */
fun encodeGrid(choice: GridChoice): String {
    val query = choice.query
    return listOfNotNull(
        "section=collections".takeIf { choice.section == Section.Collections },
        query.sort?.let { "sort=${it.value}" },
        query.played?.let { "played=${it.value}" },
        query.yearFrom?.let { "decade=$it" },
        query.genres?.takeIf { it.isNotEmpty() }?.let { "genre=" + encodeNames(it) },
        query.contentRatings?.takeIf { it.isNotEmpty() }?.let { "contentRating=" + encodeNames(it) },
        query.resolutions?.takeIf { it.isNotEmpty() }?.let { "resolution=" + it.joinToString(",") { resolution -> resolution.value } },
        query.dynamicRanges?.takeIf { it.isNotEmpty() }?.let { "dynamicRange=" + it.joinToString(",") { range -> range.value } },
        query.narratorId?.let { "narrator=" + encodeNames(listOf(it)) },
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
        sort = GridSort.of(values["sort"]) ?: GridSort.Title,
        played = WorkPlayState.of(values["played"]),
        yearFrom = decade,
        yearTo = decade?.plus(9),
        genres = decodeNames(values["genre"]),
        contentRatings = decodeNames(values["contentRating"]),
        resolutions = Resolution.entries.filter { it != Resolution.Unknown && it.value in list("resolution") }.ifEmpty { null },
        dynamicRanges = DynamicRange.entries.filter { it.value in list("dynamicRange") }.ifEmpty { null },
        narratorId = decodeNames(values["narrator"])?.singleOrNull(),
    )
    return GridChoice(query, if (values["section"] == "collections") Section.Collections else Section.Titles)
}

/** Whether a filter narrows the works. */
val GridQuery.filtered: Boolean
    get() = played != null || yearFrom != null || !genres.isNullOrEmpty() || !contentRatings.isNullOrEmpty() ||
        !resolutions.isNullOrEmpty() || !dynamicRanges.isNullOrEmpty() || narratorId != null

/** The query without its filters, keeping its sort. */
fun GridQuery.unfiltered(): GridQuery = copy(
    played = null,
    yearFrom = null,
    yearTo = null,
    genres = null,
    contentRatings = null,
    resolutions = null,
    dynamicRanges = null,
    narratorId = null,
)

/** The decades a library's [years] fall in, newest first. */
fun decadesOf(years: List<Int>): List<Int> = years.map { it - it % 10 }.distinct().sortedDescending()

// Genres can hold the "&" and "," that separate a saved grid's parts, as "Action & Adventure" does.
private fun encodeNames(names: List<String>) = names.joinToString(",") { URLEncoder.encode(it, "UTF-8") }

private fun decodeNames(text: String?) = text?.split(',')?.map { URLDecoder.decode(it, "UTF-8") }?.ifEmpty { null }

/** The grid [libraryId] showed last, among the [saved] grids, else its works by title. */
fun gridChoice(saved: Map<String, GridChoice>, libraryId: String): GridChoice = saved[libraryId] ?: decodeGrid(libraryId, "")
