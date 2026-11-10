package com.mmcdole.kivo.core.session

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import com.mmcdole.kivo.api.models.BookSeries
import com.mmcdole.kivo.api.models.BookSeriesSummary
import com.mmcdole.kivo.api.models.Collection
import com.mmcdole.kivo.api.models.CollectionSummary
import com.mmcdole.kivo.api.models.Home
import com.mmcdole.kivo.api.models.HomeRowChange
import com.mmcdole.kivo.api.models.HomeRowInfo
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.Person
import com.mmcdole.kivo.api.models.PersonSummary
import com.mmcdole.kivo.api.models.SearchResults
import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkFieldValue
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.api.models.UserState
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.Reachability
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.data.GridPage
import com.mmcdole.kivo.core.data.GridPager
import com.mmcdole.kivo.core.data.GridQuery
import com.mmcdole.kivo.core.data.GridRepository
import com.mmcdole.kivo.core.data.Heading
import com.mmcdole.kivo.core.data.HomeRepository
import com.mmcdole.kivo.core.data.Known
import com.mmcdole.kivo.core.data.LibraryFilters
import com.mmcdole.kivo.core.data.LibraryRepository
import com.mmcdole.kivo.core.data.ListsRepository
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
 * Home, the user's libraries and lists, works, people, book series and
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
    private val listWorks = ListsRepository(connection, invalidations)
    private val grid = GridRepository(connection, invalidations)
    private val userState = UserStateRepository(connection)

    /** Asks for the user's libraries again, beside the live events. */
    private val libraryRereads = MutableSharedFlow<Invalidation>(extraBufferCapacity = 1)
    private val libraryList = LibraryRepository(connection, merge(invalidations, libraryRereads), preferences)

    private val seen = Known()

    private val homeLayout = HomeRepository(connection, invalidations)

    fun home(): Flow<Load<Home>> = reachability.load(homeLayout.home().onEach(seen::read))

    /** Home's rows of one `HomeRowKind`, shown or not. */
    fun home(kind: String): Flow<Load<Home>> = reachability.load(homeLayout.home(kind))

    /** Every row of Home, in order, shown or not, read again as the layout changes. */
    fun homeRows(): Flow<Load<List<HomeRowInfo>>> = reachability.load(homeLayout.rows())

    /**
     * Sets the rows' order and which are shown, rows left out following the
     * listed ones; the layout as the server has it. [homeRows] reads it
     * again from the live event.
     */
    @Throws(Exception::class)
    suspend fun setHomeRows(rows: List<HomeRowChange>): List<HomeRowInfo> = homeLayout.setRows(rows)

    /** Forgets the order and hidden rows; the layout as the server has it. */
    @Throws(Exception::class)
    suspend fun resetHomeRows(): List<HomeRowInfo> = homeLayout.resetRows()

    /** The user's lists, My List first, read again as they change; [counted] also counts the smart lists and gives each list its first works, as a Home row has them. */
    fun lists(counted: Boolean = false): Flow<Load<List<WorkList>>> =
        reachability.load(listWorks.lists(counted).onEach { lists -> lists.forEach { seen.read(it.items.orEmpty()) } })

    /** A list's works of [kinds], or every kind when null, as a grid, a manual list's in its sort, read again in place as they change. */
    fun pagedListWorks(listId: String, kinds: List<String>?): GridPager<WorkSummary> = paged(
        "list $listId $kinds",
        listWorks.changes(),
        seen::read,
    ) { offset, limit -> listWorks.page(listId, offset, limit, kinds).let { GridPage(it.items, it.total, it.letters) } }

    @Throws(Exception::class)
    suspend fun createList(name: String): WorkList = listWorks.create(name)

    /** Makes a smart list that runs [query]. */
    @Throws(Exception::class)
    suspend fun createSmartList(name: String, query: WorkQuery): WorkList = listWorks.create(name, query)

    /** Renames a list when [name] is given and, for a smart list, replaces its rules with [query] when given, as one change. */
    @Throws(Exception::class)
    suspend fun updateList(id: String, name: String?, query: WorkQuery? = null): WorkList = listWorks.update(id, name, query)

    /** Sets the order a manual list is shown in, one of `ListSorts`, on every screen and device. */
    @Throws(Exception::class)
    suspend fun setListSort(id: String, sort: String): WorkList = listWorks.setSort(id, sort)

    /** Every work of a manual list in its custom order, whatever its sort, for a screen that moves them. */
    @Throws(Exception::class)
    suspend fun listWorksInCustomOrder(listId: String): List<WorkSummary> = listWorks.inCustomOrder(listId).also(seen::read)

    /** Puts [workIds], in that order, just after [after] in a manual list's custom order, or first when it's null, and shows the list in that order. */
    @Throws(Exception::class)
    suspend fun moveInList(listId: String, workIds: List<String>, after: String?) = listWorks.move(listId, workIds, after)

    /** How many works [query] selects, for a builder's live count. */
    @Throws(Exception::class)
    suspend fun countMatching(query: WorkQuery): Int = listWorks.countMatching(query)

    /** The fields a rule can use, for a builder. */
    @Throws(Exception::class)
    suspend fun workFields(): List<WorkField> = listWorks.fields()

    /** A field's values the user can see: those matching [search], or the ones named by [ids]. */
    @Throws(Exception::class)
    suspend fun workFieldValues(field: String, search: String?, ids: List<String>?): List<WorkFieldValue> =
        listWorks.values(field, search, ids)

    /** The names of [query]'s chosen values, by `SmartLists.nameKey`, as far as they can be read. */
    @Throws(Exception::class)
    suspend fun smartListNames(query: WorkQuery, fields: List<WorkField>): Map<String, String> = listWorks.names(query, fields)

    @Throws(Exception::class)
    suspend fun deleteList(id: String) = listWorks.delete(id)

    @Throws(Exception::class)
    suspend fun addToList(listId: String, workId: String) = listWorks.add(listId, workId)

    @Throws(Exception::class)
    suspend fun removeFromList(listId: String, workId: String) = listWorks.remove(listId, workId)

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
        grid.page(query, libraryList.kind(query.libraryId), offset, limit).also { seen.read(it.items) }

    /** Whether [libraryId] has collections, read one long. */
    @Throws(Exception::class)
    suspend fun hasCollections(libraryId: String): Boolean = grid.collections(libraryId, 0, 1).total > 0

    /**
     * The values [libraryId]'s filters offer (its works' genres, content
     * ratings, years, resolutions and dynamic ranges) and whether it has
     * collections, read again as the library changes.
     */
    fun filters(libraryId: String): Flow<Load<LibraryFilters>> = reachability.load(grid.filters(libraryId))

    /** A library's works for [query], read again in place as the library or the user's marks change. */
    fun pagedWorks(query: GridQuery): GridPager<WorkSummary> = paged("works $query", grid.changes(query.libraryId), seen::read) { offset, limit ->
        grid.page(query, libraryList.kind(query.libraryId), offset, limit).let { GridPage(it.items, it.total, it.letters) }
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

    /** Takes a work out of Continue Watching or Continue Listening until it's played again; an episode takes its series. */
    @Throws(Exception::class)
    suspend fun removeFromContinue(workId: String) = userState.removeFromContinue(workId)
}
