package com.mmcdole.kivo.core.data

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import com.mmcdole.kivo.api.models.Resolution
import com.mmcdole.kivo.core.kivoJson
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException

/** A library's grid as this device last left it: its tab, sort and filters, and whether the library had collections. */
internal data class SavedGrid(
    val tab: LibraryTab = LibraryTab.Titles,
    val sort: GridSort = GridSort.Title,
    val filters: GridFilters = GridFilters.None,
    /** Null until it's known. */
    val collections: Boolean? = null,
)

/**
 * Each library's grid as this device last left it, kept in the app's
 * preferences ([store]) by library ID, so a library opens as it was left.
 * They're read once ([load]) and at hand from then on; each change is kept
 * as it's made, in the order made.
 */
internal class SavedGrids(
    private val store: DataStore<Preferences>,
    private val scope: CoroutineScope,
) {
    private val grids = HashMap<String, SavedGrid>()
    private var loaded = false

    /**
     * Reads the saved grids, the first time it's called; a store that can't
     * be read leaves every library as new.
     */
    suspend fun load() {
        if (loaded) return
        try {
            for ((key, value) in store.data.first().asMap()) {
                val libraryId = key.name.takeIf { it.startsWith(PREFIX) }?.removePrefix(PREFIX) ?: continue
                grids[libraryId] = decode(value as? String ?: continue) ?: continue
            }
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // Each library opens as new.
        }
        loaded = true
    }

    /** [libraryId]'s grid as last left; null for one never changed. */
    fun of(libraryId: String): SavedGrid? = grids[libraryId]

    /** Keeps [libraryId]'s grid as [change] makes it from how it was last left, or from a new library's. */
    fun update(libraryId: String, change: (SavedGrid) -> SavedGrid) {
        val before = grids[libraryId]
        val now = change(before ?: SavedGrid())
        if (now == before) return
        grids[libraryId] = now
        scope.launch {
            try {
                store.edit { it[key(libraryId)] = encode(now) }
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // The grid still shows; it just isn't kept.
            }
        }
    }

    internal companion object {
        private const val PREFIX = "kivo.grid."

        fun key(libraryId: String) = stringPreferencesKey(PREFIX + libraryId)

        fun encode(grid: SavedGrid): String {
            val filters = grid.filters.chosen
            val narrator = filters.firstNotNullOfOrNull { it as? GridFilter.Narrator }
            return kivoJson.encodeToString(
                StoredGrid.serializer(),
                StoredGrid(
                    tab = grid.tab.value,
                    sort = grid.sort.value,
                    played = grid.filters.played?.value,
                    decade = filters.firstNotNullOfOrNull { (it as? GridFilter.Decade)?.year },
                    genres = filters.mapNotNull { (it as? GridFilter.Genre)?.name },
                    contentRatings = filters.mapNotNull { (it as? GridFilter.ContentRating)?.name },
                    resolutions = filters.mapNotNull { (it as? GridFilter.Resolution)?.resolution?.value },
                    dynamicRanges = filters.mapNotNull { (it as? GridFilter.DynamicRange)?.range?.value },
                    narratorId = narrator?.id,
                    narratorName = narrator?.name,
                    collections = grid.collections,
                ),
            )
        }

        /** The grid [encode] wrote; null for text that isn't one. */
        fun decode(text: String): SavedGrid? = try {
            kivoJson.decodeFromString(StoredGrid.serializer(), text).grid()
        } catch (_: SerializationException) {
            null
        }
    }
}

/** A saved grid as it's stored, each value as the server names it. */
@Serializable
internal class StoredGrid(
    val tab: String? = null,
    val sort: String? = null,
    val played: String? = null,
    val decade: Int? = null,
    val genres: List<String> = emptyList(),
    val contentRatings: List<String> = emptyList(),
    val resolutions: List<String> = emptyList(),
    val dynamicRanges: List<String> = emptyList(),
    val narratorId: String? = null,
    val narratorName: String? = null,
    val collections: Boolean? = null,
) {
    /** The grid stored, a value this version doesn't know left out: the works' tab and the title sort stand in for theirs. */
    fun grid(): SavedGrid {
        val filters = buildList {
            WorkPlayState.of(played)?.let { add(GridFilter.Played(it)) }
            decade?.let { add(GridFilter.Decade(it)) }
            genres.forEach { add(GridFilter.Genre(it)) }
            contentRatings.forEach { add(GridFilter.ContentRating(it)) }
            resolutions.map(Resolution::of).filter { it != Resolution.Unknown }.forEach { add(GridFilter.Resolution(it)) }
            dynamicRanges.mapNotNull(DynamicRange::of).forEach { add(GridFilter.DynamicRange(it)) }
            narratorId?.let { add(GridFilter.Narrator(it, narratorName)) }
        }
        return SavedGrid(LibraryTab.of(tab) ?: LibraryTab.Titles, GridSort.of(sort) ?: GridSort.Title, GridFilters.of(filters), collections)
    }
}
