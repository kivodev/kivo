package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.BookSeriesSummaryPage
import com.mmcdole.kivo.api.models.CollectionSummaryPage
import com.mmcdole.kivo.api.models.LibraryFacets
import com.mmcdole.kivo.api.models.PersonSummaryPage
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.map

/**
 * A library's grids: its works, sorted and filtered, and its collections,
 * authors and book series. Each is paged by offset so a grid can load the
 * pages around any scroll position: every page carries the total, and the
 * first, in letter order, the letter rail's starts.
 */
internal class GridRepository(private val connection: ServerConnection, private val invalidations: Flow<Invalidation>) {
    /** [limit] works of [query]'s grid from [offset]. */
    suspend fun page(query: GridQuery, offset: Int, limit: Int): WorkSummaryPage = connection.libraries.listLibraryWorks(
        id = query.libraryId,
        sort = query.sort,
        played = query.played,
        genre = query.genres,
        yearFrom = query.yearFrom,
        yearTo = query.yearTo,
        contentRating = query.contentRatings,
        resolution = query.resolutions,
        dynamicRange = query.dynamicRanges,
        offset = offset.takeIf { it > 0 },
        limit = limit,
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
    fun facets(libraryId: String): Flow<LibraryFacets> =
        refreshing(invalidations, { it is Invalidation.WorkChanged || (it is Invalidation.LibraryChanged && libraryId in it.libraryIds) }) {
            connection.libraries.getLibraryFacets(libraryId).result()
        }

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

/** A library's works grid's sort and filters, as `listLibraryWorks` takes them. */
data class GridQuery(
    val libraryId: String,
    /** A `WorkSort`, such as `title` or `addedAt`; null for the server's default. */
    val sort: String? = null,
    /** A `PlayedFilter`: `unplayed`, `inProgress` or `played`. */
    val played: String? = null,
    val genres: List<String>? = null,
    val yearFrom: Int? = null,
    val yearTo: Int? = null,
    val contentRatings: List<String>? = null,
    /** `Resolution`s: `4k`, `1080p`, `720p`, `sd`. */
    val resolutions: List<String>? = null,
    /** `DynamicRange`s: `hdr`, `dolbyVision`. */
    val dynamicRanges: List<String>? = null,
)
