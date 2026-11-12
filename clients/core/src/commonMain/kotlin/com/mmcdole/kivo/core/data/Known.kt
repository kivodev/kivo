package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.BookSeries
import com.mmcdole.kivo.api.models.BookSeriesSummary
import com.mmcdole.kivo.api.models.Collection
import com.mmcdole.kivo.api.models.CollectionSummary
import com.mmcdole.kivo.api.models.Home
import com.mmcdole.kivo.api.models.ImageRef
import com.mmcdole.kivo.api.models.Person
import com.mmcdole.kivo.api.models.PersonSummary
import com.mmcdole.kivo.api.models.SearchResults
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkSummary

/**
 * A person's or book series' page as the lists read so far name it, for the
 * page to show before it's read: the name, and the photo or cover a list
 * showed with it, if any.
 */
data class Heading(val id: String, val name: String, val image: ImageRef? = null)

/** The work's summary: its page's fields that every list shows too. */
fun WorkDetail.summary(): WorkSummary = WorkSummary(
    id = id, kind = kind, title = title, matchState = matchState, available = available, images = images,
    year = year, durationMs = durationMs, communityRating = communityRating, communityVotes = communityVotes,
    contentRating = contentRating, addedAt = addedAt, userState = userState, movie = movie, series = series,
    season = season, episode = episode, extra = extra, book = book, libraryId = libraryId,
)

/**
 * The version the work's page selects before any choice: the one the user
 * last played while the work still has it, else the first. The server
 * lists versions in order and plays this same version when a play names
 * none.
 */
internal fun WorkDetail.defaultVersion(): Version? = versions.firstOrNull { it.id == userState?.versionId } ?: versions.firstOrNull()

/** The version [versionId] names, else the [defaultVersion]. */
fun WorkDetail.selectedVersion(versionId: String?): Version? = versions.firstOrNull { it.id == versionId } ?: defaultVersion()

/**
 * Where the work resumes when the version [versionId] selects plays (see
 * [selectedVersion]), or null to start from the beginning. A position
 * belongs to the edition of the version it was saved on, so a version of
 * another edition starts from the beginning.
 */
fun WorkDetail.resumeAt(versionId: String?): Int? {
    val position = userState?.positionMs
    val saved = versions.firstOrNull { it.id == userState?.versionId }
    val selected = selectedVersion(versionId)
    return if (saved != null && selected != null && saved.edition != selected.edition) null else position
}

/**
 * What the session has read lately, so a page shows at once what's already
 * known of what it opens while it reads the rest: the summaries of every
 * list and page read (the last [SUMMARIES]), the last [PAGES] work, person,
 * book series and collection pages, the names of the people and book series
 * they name, the collections' cards, and the first pages of the last
 * [GRIDS] grids.
 * Each is the newest read of it, held only while the session is open.
 * Called on the main thread, as the session's work runs.
 */
internal class Known {
    private val summaries = Recent<WorkSummary>(SUMMARIES)
    private val works = Recent<WorkDetail>(PAGES)
    private val people = Recent<Person>(PAGES)
    private val series = Recent<BookSeries>(PAGES)
    private val collections = Recent<Collection>(PAGES)
    private val collectionCards = Recent<CollectionSummary>(SUMMARIES)
    private val personNames = Recent<Heading>(SUMMARIES)
    private val seriesNames = Recent<Heading>(SUMMARIES)
    private val gridStarts = Recent<GridPage>(GRIDS)

    fun summary(id: String): WorkSummary? = summaries[id]

    fun work(id: String): WorkDetail? = works[id]

    fun person(id: String): Person? = people[id]

    fun bookSeries(id: String): BookSeries? = series[id]

    fun collection(id: String): Collection? = collections[id]

    fun collectionCard(id: String): CollectionSummary? = collectionCards[id]

    fun personHeading(id: String): Heading? = people[id]?.let { Heading(it.id, it.name, it.photo) } ?: personNames[id]

    fun bookSeriesHeading(id: String): Heading? = series[id]?.let { Heading(it.id, it.name, it.cover()) } ?: seriesNames[id]

    /** The first page of the grid [key] names, as last read. */
    fun gridStart(key: String): GridPage? = gridStarts[key]

    /** Keeps [page] as the first page of the grid [key] names. */
    fun readGridStart(key: String, page: GridPage) = gridStarts.put(listOf(key to page))

    /** A grid's page: its works, collections, authors or book series. */
    fun readGrid(items: List<GridItem>) {
        read(items.mapNotNull { (it as? GridItem.Work)?.work })
        readCollections(items.mapNotNull { (it as? GridItem.Collection)?.collection })
        readPeople(items.mapNotNull { (it as? GridItem.Author)?.author })
        readBookSeries(items.mapNotNull { (it as? GridItem.BookSeries)?.series })
    }

    fun read(list: List<WorkSummary>) {
        summaries.put(list.map { it.id to it })
        names(
            people = list.flatMap { summary -> summary.book?.let { it.authors + it.narrators }.orEmpty().map { Heading(it.id, it.name) } },
            series = list.flatMap { summary -> summary.book?.series.orEmpty().map { Heading(it.id, it.name) } },
        )
    }

    /** An audiobooks library's authors. */
    fun readPeople(list: List<PersonSummary>) = names(people = list.map { Heading(it.id, it.name, it.photo) })

    /** An audiobooks library's book series. */
    fun readBookSeries(list: List<BookSeriesSummary>) = names(series = list.map { Heading(it.id, it.name, it.cover) })

    fun readCollections(list: List<CollectionSummary>) = collectionCards.put(list.map { it.id to it })

    fun read(collection: Collection) {
        collections.put(listOf(collection.id to collection))
        read(collection.items)
    }

    fun read(home: Home) = read(home.rows.flatMap { it.items })

    fun read(results: SearchResults) {
        read(results.works)
        readPeople(results.people)
        readCollections(results.collections)
    }

    fun read(work: WorkDetail) {
        works.put(listOf(work.id to work))
        read(listOf(work.summary()) + work.extras + listOfNotNull(work.series?.nextUp))
        readCollections(work.collections)
        names(people = work.credits.map { Heading(it.personId, it.name, it.photo) })
    }

    fun read(person: Person) {
        people.put(listOf(person.id to person))
        read(person.works + person.standalone + person.narrated + person.bookSeries.flatMap { it.items })
        names(series = person.bookSeries.map { Heading(it.id, it.name, it.cover()) })
    }

    fun read(bookSeries: BookSeries) {
        series.put(listOf(bookSeries.id to bookSeries))
        read(bookSeries.items)
    }

    /** Keeps names, each with the picture it was last seen with when this read has none. */
    private fun names(people: List<Heading> = emptyList(), series: List<Heading> = emptyList()) {
        personNames.put(people.map { it.id to it }) { old, new -> if (new.image == null && old.image != null) new.copy(image = old.image) else new }
        seriesNames.put(series.map { it.id to it }) { old, new -> if (new.image == null && old.image != null) new.copy(image = old.image) else new }
    }

    private fun BookSeries.cover() = items.firstOrNull()?.images?.poster

    private companion object {
        const val SUMMARIES = 1_000
        const val PAGES = 20
        const val GRIDS = 8
    }
}

/**
 * The last [capacity] values put, by ID, the oldest going as new ones come;
 * called on the main thread, as the session's work runs.
 */
private class Recent<V : Any>(private val capacity: Int) {
    private val values = LinkedHashMap<String, V>()

    operator fun get(id: String): V? = values[id]

    /** Puts [entries], each [merge]d with the value it replaces. */
    fun put(entries: List<Pair<String, V>>, merge: (old: V, new: V) -> V = { _, new -> new }) {
        for ((id, value) in entries) {
            val replaced = values.remove(id)
            values[id] = if (replaced == null) value else merge(replaced, value)
        }
        val oldest = values.keys.iterator()
        while (values.size > capacity) {
            oldest.next()
            oldest.remove()
        }
    }
}
