package com.mmcdole.kivo.core.data

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import io.ktor.http.decodeURLQueryComponent

// Migration (grid-choice): remove after every client has updated past this release.
// This file, each platform's platformGridDefaults, and SavedGrids' use of them.

/**
 * A library's grid as an earlier Apple app kept it in the user defaults:
 * its tab, sort and filters by name ([values]), and whether the library
 * had collections.
 */
internal class EarlierGridDefault(val values: Map<String, String>, val collections: Boolean?)

/**
 * The grids an earlier version of the app kept outside the app's
 * preferences, by library ID ([read]), and forgetting them once they're
 * moved ([forget]).
 */
internal class EarlierGridDefaults(val read: () -> Map<String, EarlierGridDefault>, val forget: () -> Unit) {
    companion object {
        val None = EarlierGridDefaults({ emptyMap() }, {})
    }
}

/** Where this platform's earlier app kept its grids outside the app's preferences. */
internal expect val platformGridDefaults: EarlierGridDefaults

/**
 * Moves the grids an earlier version saved to where [SavedGrids] keeps
 * them, once: an Android app's from [store] itself, an Apple app's from
 * [earlier]. A library saved since keeps what it has. Returns the
 * preferences as they are after.
 */
internal suspend fun moveEarlierGrids(store: DataStore<Preferences>, earlier: EarlierGridDefaults): Preferences {
    val defaults = earlier.read()
    val moved = store.edit { preferences ->
        for ((key, value) in preferences.asMap().toList()) {
            val libraryId = key.name.takeIf { it.startsWith(ANDROID_PREFIX) }?.removePrefix(ANDROID_PREFIX) ?: continue
            preferences.remove(key)
            if (value is String && preferences[SavedGrids.key(libraryId)] == null) {
                preferences[SavedGrids.key(libraryId)] = SavedGrids.encode(earlierAndroidGrid(value))
            }
        }
        for ((libraryId, default) in defaults) {
            if (preferences[SavedGrids.key(libraryId)] == null) preferences[SavedGrids.key(libraryId)] = SavedGrids.encode(earlierAppleGrid(default))
        }
    }
    if (defaults.isNotEmpty()) earlier.forget()
    return moved
}

/** An earlier Android app's saved grids, each under this and its library's ID. */
private const val ANDROID_PREFIX = "grid."

/**
 * The grid an earlier Android app saved as
 * `section=collections&sort=addedAt&resolution=4k,1080p&narrator=p1`, its
 * names form-encoded.
 */
internal fun earlierAndroidGrid(text: String): SavedGrid {
    val values = text.split('&').mapNotNull { part -> part.split('=', limit = 2).takeIf { it.size == 2 }?.let { it[0] to it[1] } }.toMap()
    fun names(key: String) = values[key]?.split(',').orEmpty().map { it.decodeURLQueryComponent(plusIsSpace = true) }
    return StoredGrid(
        tab = values["section"],
        sort = earlierSort(values["sort"]),
        played = values["played"],
        decade = values["decade"]?.toIntOrNull(),
        genres = names("genre"),
        contentRatings = names("contentRating"),
        resolutions = values["resolution"]?.split(',').orEmpty(),
        // Migration (api-naming): remove after every client has updated past this release.
        dynamicRanges = (values["dynamicRange"] ?: values["hdr"])?.split(',').orEmpty(),
        narratorId = names("narrator").singleOrNull(),
    ).grid()
}

/**
 * The grid an earlier Apple app kept: its genres and content ratings a
 * line each, its resolutions and dynamic ranges between commas.
 */
internal fun earlierAppleGrid(default: EarlierGridDefault): SavedGrid {
    val values = default.values
    fun list(key: String, separator: Char) = values[key]?.split(separator).orEmpty().filter { it.isNotEmpty() }
    return StoredGrid(
        tab = values["tab"],
        sort = earlierSort(values["sort"]),
        played = values["played"],
        decade = values["decade"]?.toIntOrNull(),
        genres = list("genres", '\n'),
        contentRatings = list("contentRatings", '\n'),
        resolutions = list("resolutions", ','),
        // Migration (api-naming): remove after every client has updated past this release.
        dynamicRanges = list(if ("dynamicRanges" in values) "dynamicRanges" else "hdr", ','),
        narratorId = values["narrator"],
        narratorName = values["narratorName"]?.takeIf { it.isNotEmpty() },
        collections = default.collections,
    ).grid()
}

// Migration (api-naming): remove after every client has updated past this release.
/** The last-episode sort, once saved as `lastEpisodeAdded`. */
private fun earlierSort(sort: String?) = if (sort == "lastEpisodeAdded") GridSort.LastEpisodeAddedAt.value else sort
