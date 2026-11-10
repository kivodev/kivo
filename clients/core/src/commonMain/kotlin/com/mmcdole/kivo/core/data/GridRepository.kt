package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.BookSeriesSummaryPage
import com.mmcdole.kivo.api.models.CollectionSummaryPage
import com.mmcdole.kivo.api.models.PersonSummaryPage
import com.mmcdole.kivo.api.models.WorkQueryRequest
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.map

/**
 * A library's grids: its works, sorted and filtered, and its collections,
 * authors and book series. Each is paged by offset so a grid can load the
 * pages around any scroll position: the first page carries the total and,
 * in letter order, the letter rail's starts.
 */
internal class GridRepository(private val connection: ServerConnection, private val invalidations: Flow<Invalidation>) {
    /** [limit] works of [query]'s grid from [offset], its library being of [libraryKind]. */
    suspend fun page(query: GridQuery, libraryKind: String?, offset: Int, limit: Int): WorkSummaryPage = connection.works.queryWorks(
        WorkQueryRequest(SmartLists.gridQuery(query, libraryKind), library = query.libraryId, offset = offset.takeIf { it > 0 }, limit = limit),
    ).result()

    /** [limit] of [libraryId]'s collections from [offset], by name. */
    suspend fun collections(libraryId: String, offset: Int, limit: Int): CollectionSummaryPage =
        connection.libraries.listLibraryCollections(libraryId, offset.takeIf { it > 0 }, limit).result()

    /** [limit] of an audiobooks library's authors from [offset], by name. */
    suspend fun authors(libraryId: String, offset: Int, limit: Int): PersonSummaryPage =
        connection.libraries.listLibraryAuthors(libraryId, offset.takeIf { it > 0 }, limit).result()

    /** [limit] of an audiobooks library's book series from [offset], by name. */
    suspend fun bookSeries(libraryId: String, offset: Int, limit: Int): BookSeriesSummaryPage =
        connection.libraries.listLibraryBookSeries(libraryId, offset.takeIf { it > 0 }, limit).result()

    /** What [libraryId]'s filters offer, and whether it has collections, read again as the library or its works change. */
    fun filters(libraryId: String): Flow<LibraryFilters> =
        refreshing(invalidations, { it is Invalidation.WorkChanged || (it is Invalidation.LibraryChanged && libraryId in it.libraryIds) }) {
            coroutineScope {
                val values = FILTER_FIELDS.map { field -> async { values(field, libraryId) } }
                val collections = async { collections(libraryId, 0, 1).total > 0 }
                val (genres, years, contentRatings, resolutions, dynamicRanges) = values.map { it.await() }
                LibraryFilters(genres, years.mapNotNull { it.toIntOrNull() }, contentRatings, resolutions, dynamicRanges, collections.await())
            }
        }

    /** The values of [field] that [libraryId]'s works have. */
    private suspend fun values(field: String, libraryId: String): List<String> =
        connection.works.listWorkFieldValues(field, library = libraryId).result().items.map { it.value }

    /** Emits when [libraryId]'s grids changed: reload the loaded pages, keeping the place. */
    fun changes(libraryId: String): Flow<Unit> = invalidations
        .filter {
            it == Invalidation.Everything ||
                it is Invalidation.UserStateChanged ||
                it is Invalidation.WorkChanged ||
                (it is Invalidation.LibraryChanged && libraryId in it.libraryIds)
        }
        .map { }
}

/** The fields of a library's filters, in [LibraryFilters]' order. */
private val FILTER_FIELDS = listOf("genre", "year", "contentRating", "resolution", "dynamicRange")

/** What a library's filters offer, of the values its works have, and whether it has collections. */
data class LibraryFilters(
    /** By name. */
    val genres: List<String>,
    /** Release years, oldest first. */
    val years: List<Int>,
    /** In the order to show them. */
    val contentRatings: List<String>,
    /** `Resolution`s, largest first. */
    val resolutions: List<String>,
    /** `hdr` and `dolbyVision`. */
    val dynamicRanges: List<String>,
    val collections: Boolean,
)

/** A library's works grid's sort and filters. */
data class GridQuery(
    val libraryId: String,
    /** A sortable field of a work query, such as `title` or `addedAt`, in its own direction; null for `title`. */
    val sort: String? = null,
    /** A `playState`: `unplayed`, `inProgress` or `played`. */
    val played: String? = null,
    val genres: List<String>? = null,
    val yearFrom: Int? = null,
    val yearTo: Int? = null,
    val contentRatings: List<String>? = null,
    /** `Resolution`s: `4k`, `1080p`, `720p`, `sd`. */
    val resolutions: List<String>? = null,
    /** `hdr`, `dolbyVision`. */
    val dynamicRanges: List<String>? = null,
)
