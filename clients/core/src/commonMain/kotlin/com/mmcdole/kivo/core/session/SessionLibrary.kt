package com.mmcdole.kivo.core.session

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import com.mmcdole.kivo.api.models.BookSeries
import com.mmcdole.kivo.api.models.BookSeriesSummary
import com.mmcdole.kivo.api.models.Collection
import com.mmcdole.kivo.api.models.CollectionSummary
import com.mmcdole.kivo.api.models.Home
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryFacets
import com.mmcdole.kivo.api.models.Person
import com.mmcdole.kivo.api.models.PersonSummary
import com.mmcdole.kivo.api.models.SearchResults
import com.mmcdole.kivo.api.models.UserState
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.Reachability
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.data.FavoritesRepository
import com.mmcdole.kivo.core.data.GridPage
import com.mmcdole.kivo.core.data.GridPager
import com.mmcdole.kivo.core.data.GridQuery
import com.mmcdole.kivo.core.data.GridRepository
import com.mmcdole.kivo.core.data.Heading
import com.mmcdole.kivo.core.data.HomeRepository
import com.mmcdole.kivo.core.data.Known
import com.mmcdole.kivo.core.data.LibraryRepository
import com.mmcdole.kivo.core.data.SearchRepository
import com.mmcdole.kivo.core.data.UserStateRepository
import com.mmcdole.kivo.core.data.WorkRepository
import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.emitAll
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.merge
import kotlinx.coroutines.flow.onEach

/**
 * What the session reads of the server's libraries ([KivoSession.library]):
 * Home, the user's libraries, favorites, works, people, book series and
 * collections, the grids, search, and the user's marks on works. Each read
 * is a Flow of [Load], read again as the live events say and asked again
 * while the server can't be reached; what it has read lately shows at once
 * ([known]).
 */
class SessionLibrary internal constructor(
    private val connection: ServerConnection,
    private val invalidations: Flow<Invalidation>,
    private val reachability: Reachability,
    private val scope: CoroutineScope,
    preferences: DataStore<Preferences>,
) {
    private val works = WorkRepository(connection, invalidations)
    private val favoriteWorks = FavoritesRepository(connection, invalidations)
    private val grid = GridRepository(connection, invalidations)
    private val userState = UserStateRepository(connection)

    /** Asks for the user's libraries again, beside the live events. */
    private val libraryRereads = MutableSharedFlow<Invalidation>(extraBufferCapacity = 1)
    private val libraryList = LibraryRepository(connection, merge(invalidations, libraryRereads), preferences)

    private val seen = Known()

    fun home(): Flow<Load<Home>> = reachability.load(HomeRepository(connection, invalidations).home().onEach(seen::read))

    /** The first page of one `WorkKind`'s favorites, [limit] long or the server's page, most recently marked first; its `total` says whether there are any. */
    fun favorites(kind: String, limit: Int?): Flow<Load<WorkSummaryPage>> =
        reachability.load(favoriteWorks.favorites(kind, limit).onEach { seen.read(it.items) })

    /** How many works the user marked a favorite, as it changes. */
    fun favoritesCount(): Flow<Load<Int>> = reachability.load(favoriteWorks.count())

    /** One kind's favorites as a grid, most recently marked first, read again in place as they change. */
    fun pagedFavorites(kind: String): GridPager<WorkSummary> = paged(
        "favorites $kind",
        favoriteWorks.changes(),
        seen::read,
    ) { offset, limit -> favoriteWorks.page(kind, offset, limit).let { GridPage(it.items, it.total, it.letters) } }

    /**
     * The libraries the user sees, in their order, read again as they change
     * and on [rereadLibraries]; as last read on this device at once, so they
     * show before the server answers.
     */
    fun libraries(): Flow<Load<List<Library>>> = flow {
        emitAll(reachability.load(libraryList.libraries(), libraryList.kept()))
    }

    /**
     * Reads the user's libraries again, for a change the live events don't
     * announce, such as a pin changed elsewhere; [SessionAccount.setLibraries]
     * does it too.
     */
    fun rereadLibraries() {
        libraryRereads.tryEmit(Invalidation.Everything)
    }

    /** A work's page; as read earlier in the session at once, when it was. */
    fun work(id: String): Flow<Load<WorkDetail>> = flow {
        emitAll(reachability.load(works.work(id).onEach(seen::read), seen.work(id)))
    }

    /**
     * What the session has read of a work, from its summary in any list read
     * lately or its page, for the page to show at once while [work] reads
     * the rest; null when it has read neither.
     */
    fun known(id: String): WorkSummary? = seen.summary(id)

    /** A person's name and photo, as the lists and pages read lately named them; null when none did. */
    fun knownPerson(id: String): Heading? = seen.personHeading(id)

    /** A book series' name and cover, as the lists and pages read lately named it; null when none did. */
    fun knownBookSeries(id: String): Heading? = seen.bookSeriesHeading(id)

    /** A collection's card, as the grids and pages read lately showed it; null when none did. */
    fun knownCollection(id: String): CollectionSummary? = seen.collectionCard(id)

    /** A person's page, for a search result, a credit or an author of a library's. */
    fun person(id: String): Flow<Load<Person>> = flow {
        emitAll(reachability.load(works.person(id).onEach(seen::read), seen.person(id)))
    }

    /** A book series' page, for a book's series or a library's. */
    fun bookSeries(id: String): Flow<Load<BookSeries>> = flow {
        emitAll(reachability.load(works.bookSeries(id).onEach(seen::read), seen.bookSeries(id)))
    }

    /** A collection's page with its root works; as read earlier in the session at once, when it was. */
    fun collection(id: String): Flow<Load<Collection>> = flow {
        emitAll(reachability.load(works.collection(id).onEach(seen::read), seen.collection(id)))
    }

    /** A series' seasons or a season's episodes. */
    fun children(workId: String): Flow<Load<List<WorkSummary>>> =
        reachability.load(works.children(workId).onEach(seen::read))

    /** [limit] works of a library's grid from [offset], such as one long to ask whether a filter finds any. */
    @Throws(Exception::class)
    suspend fun gridPage(query: GridQuery, offset: Int, limit: Int): WorkSummaryPage =
        grid.page(query, offset, limit).also { seen.read(it.items) }

    /** Whether [libraryId] has collections, read one long. */
    @Throws(Exception::class)
    suspend fun hasCollections(libraryId: String): Boolean = grid.collections(libraryId, 0, 1).total > 0

    /**
     * The values [libraryId]'s filters offer (its works' genres, content
     * ratings, years, resolutions and dynamic ranges) and whether it has
     * collections, read again as the library changes.
     */
    fun facets(libraryId: String): Flow<Load<LibraryFacets>> = reachability.load(grid.facets(libraryId))

    /** A library's works for [query], read again in place as the library or the user's marks change. */
    fun pagedWorks(query: GridQuery): GridPager<WorkSummary> = paged("works $query", grid.changes(query.libraryId), seen::read) { offset, limit ->
        grid.page(query, offset, limit).let { GridPage(it.items, it.total, it.letters) }
    }

    /** A library's collections, by name, read again in place as the library changes. */
    fun pagedCollections(libraryId: String): GridPager<CollectionSummary> =
        paged("collections $libraryId", grid.changes(libraryId), seen::readCollections) { offset, limit ->
            grid.collections(libraryId, offset, limit).let { GridPage(it.items, it.total, it.letters) }
        }

    /** An audiobooks library's authors, by name, read again in place as the library changes. */
    fun pagedAuthors(libraryId: String): GridPager<PersonSummary> =
        paged("authors $libraryId", grid.changes(libraryId), seen::readPeople) { offset, limit ->
            grid.authors(libraryId, offset, limit).let { GridPage(it.items, it.total, it.letters) }
        }

    /** An audiobooks library's book series, by name, read again in place as the library changes. */
    fun pagedBookSeries(libraryId: String): GridPager<BookSeriesSummary> =
        paged("bookSeries $libraryId", grid.changes(libraryId), seen::readBookSeries) { offset, limit ->
            grid.bookSeries(libraryId, offset, limit).let { GridPage(it.items, it.total, it.letters) }
        }

    private fun <T : Any> paged(key: String, changes: Flow<Unit>, keep: (List<T>) -> Unit, read: suspend (Int, Int) -> GridPage<T>) =
        GridPager(read, changes, reachability, seen, key, keep, scope, Dispatchers.Default)

    /** The works, people and collections matching [query], each best first, [limit] of each or the server's number. */
    @Throws(Exception::class)
    suspend fun search(query: String, limit: Int?): SearchResults =
        SearchRepository(connection).search(query, limit).also(seen::read)

    @Throws(Exception::class)
    suspend fun setPlayed(workId: String, played: Boolean): UserState = userState.setPlayed(workId, played)

    @Throws(Exception::class)
    suspend fun setFavorite(workId: String, favorite: Boolean): UserState = userState.setFavorite(workId, favorite)

    /** Takes a work out of Continue Watching or Continue Listening until it's played again; an episode takes its series. */
    @Throws(Exception::class)
    suspend fun removeFromContinue(workId: String) = userState.removeFromContinue(workId)
}
