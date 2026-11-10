package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.LetterStart
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.Reachability
import com.mmcdole.kivo.core.RequestProblem
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlin.coroutines.CoroutineContext
import kotlin.math.abs

/**
 * A page of a grid as the server answers it: its items, the grid's total,
 * which a grid of works has on its first page only, and on the first page
 * in letter order the letter rail's starts.
 */
internal class GridPage<T : Any>(val items: List<T>, val total: Int?, val letters: List<LetterStart>?)

/**
 * A grid too long to read at once, such as a library's works or its
 * authors, paged by position: the pages a client asks for ([load]) as its grid comes to rest
 * on them, each read once at its offset, and kept only around where it
 * rests. It holds no
 * layout, only which pages are read and that they're of one result.
 *
 * The first page brings the count and the letter rail's starts. It shows at
 * once when the session read it lately, and is read again either way. A
 * page that gets no answer is asked again after 1 s, 2 s, 4 s, 8 s and
 * 16 s, and then stays failed until it's asked for again; any refusal is
 * final. Pages the client
 * no longer asks for stop being read, and an answer for a grid since read
 * again is dropped. As the live events say the grid changed, the pages
 * asked for last are read again in place, each showing what it had until
 * its answer comes.
 *
 * Called on the main thread, as the core's work runs; [close] it when the
 * grid goes.
 */
class GridPager<T : Any> internal constructor(
    private val read: suspend (offset: Int, limit: Int) -> GridPage<T>,
    changes: Flow<Unit>,
    private val reachability: Reachability,
    private val known: Known,
    /** Names the grid for its first page as the session last read it. */
    private val key: String,
    /** Keeps what each answer names, for the pages it opens. */
    private val seen: (List<T>) -> Unit,
    scope: CoroutineScope,
    /** Where answers are read, off the main thread. */
    private val work: CoroutineContext,
    private val pageSize: Int = PAGE_SIZE,
    private val kept: Int = KEPT_PAGES,
) {
    private val scope = CoroutineScope(scope.coroutineContext + SupervisorJob(scope.coroutineContext[Job]))

    /** Each page's read; on [pages] only as it's published. */
    private val pages = HashMap<Int, Load<List<T>>>()
    private var total: Int? = null
    private var letters: List<LetterStart> = emptyList()

    /** Counts the reads of the whole grid: an answer for an earlier one is dropped. */
    private var generation = 0

    /** The pages being read, for this generation. */
    private val reading = HashMap<Int, Job>()

    /** The pages the client asked for last. */
    private var window = 0..0

    /** Completes as this generation's first page answers or fails. */
    private var firstAnswer = CompletableDeferred<Unit>()

    private val mutableState = MutableStateFlow(GridPages<T>(null, emptyList(), emptyMap(), pageSize))

    /** The grid as read so far. */
    val state: StateFlow<GridPages<T>> = mutableState.asStateFlow()

    init {
        known.gridStart<T>(key)?.let { first ->
            total = first.total
            letters = first.letters.orEmpty()
            pages[0] = Load.Loaded(first.items)
        }
        start(0)
        publish()
        this.scope.launch { changes.collect { reload() } }
    }

    /**
     * Reads the pages holding the items from [first] to [last] that aren't
     * read or being read, a failed one again; pages being read outside them
     * stop, and those far from them are let go. A client asks for those in
     * view and a screen past them as its grid comes to rest.
     */
    fun load(first: Int, last: Int) {
        val count = total ?: return
        if (count == 0 || last < first) return
        val wanted = first.coerceIn(0, count - 1) / pageSize..last.coerceIn(0, count - 1) / pageSize
        window = wanted
        for ((page, job) in reading.toList()) {
            if (page == 0 || page in wanted) continue
            job.cancel()
            reading.remove(page)
            if (pages[page] !is Load.Loaded) pages.remove(page)
        }
        for (page in wanted) if (page !in reading && pages[page] !is Load.Loaded) start(page)
        letGo()
        publish()
    }

    /**
     * Reads the grid again in place: the first page and those asked for
     * last; returns once the first page answers or fails, as for a pull to
     * refresh or Try again.
     */
    @Throws(Exception::class)
    suspend fun refresh() {
        reload()
        firstAnswer.await()
    }

    /** Stops reading: the grid is gone. */
    fun close() {
        scope.cancel()
    }

    private fun reload() {
        generation++
        reading.values.forEach { it.cancel() }
        reading.clear()
        firstAnswer = CompletableDeferred()
        pages.keys.retainAll { it == 0 || it in window }
        start(0)
        val count = total
        if (count != null) for (page in window) if (page != 0 && page * pageSize < count) start(page)
        publish()
    }

    private fun start(page: Int) {
        val generation = generation
        if (pages[page] !is Load.Loaded) pages[page] = Load.Loading
        reading[page] = scope.launch {
            val job = currentCoroutineContext()[Job]
            try {
                val answer = reachability.retrying(RETRIES, failed = { if (generation == this@GridPager.generation) failed(page, it) }) {
                    withContext(work) { read(page * pageSize, pageSize) }
                }
                if (generation == this@GridPager.generation) answered(page, answer)
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // Shown as failed as it failed.
            } finally {
                if (reading[page] === job) reading.remove(page)
            }
        }
    }

    private fun answered(page: Int, answer: GridPage<T>) {
        seen(answer.items)
        if (page == 0) {
            known.readGridStart(key, answer)
            val count = answer.total ?: answer.items.size
            total = count
            letters = answer.letters.orEmpty()
            pages.keys.removeAll { it * pageSize >= count && it != 0 }
            firstAnswer.complete(Unit)
        }
        // A page that reads as it was keeps its value, so a client sees nothing new to draw.
        if (pages[page]?.getOrNull() != answer.items) pages[page] = Load.Loaded(answer.items)
        letGo()
        publish()
    }

    /** A read failed: the page shows why, unless it has an answer already. */
    private fun failed(page: Int, problem: RequestProblem) {
        if (page == 0) firstAnswer.complete(Unit)
        if (pages[page] is Load.Loaded) return
        pages[page] = Load.Failed(problem)
        publish()
    }

    /** Lets the pages farthest from those asked for go, keeping [kept] and the first. */
    private fun letGo() {
        val far = pages.keys.filter { it != 0 && it !in window }
        val over = pages.size - kept
        if (over <= 0 || far.isEmpty()) return
        val middle = (window.first + window.last) / 2
        far.sortedByDescending { abs(it - middle) }.take(over).forEach { pages.remove(it) }
    }

    /** Publishes the grid as read so far, unless it's as last published, as when a client asks again for pages it has. */
    private fun publish() {
        val shown = mutableState.value
        if (shown.total == total && shown.letters == letters && shown.pages == pages) return
        mutableState.value = GridPages(total, letters, pages.toMap(), pageSize)
    }

    internal companion object {
        const val PAGE_SIZE = 60

        /** About two thousand items, the window and the screens around it. */
        const val KEPT_PAGES = 34

        const val RETRIES = 5
    }
}

/**
 * A [GridPager] grid as read so far: how many items it has, null until
 * the first page answers, the letter rail's starts, and each page's read,
 * by number, a page holding the items from its number times [pageSize].
 */
class GridPages<T : Any> internal constructor(
    val total: Int?,
    /** Where each letter starts, for a list in letter order; empty for others. */
    val letters: List<LetterStart>,
    val pages: Map<Int, Load<List<T>>>,
    val pageSize: Int,
) {
    /** Why the grid can't be shown: its first page failed and there's no count. */
    val problem: RequestProblem? get() = if (total == null) pages[0]?.problemOrNull() else null

    /** Page [page]'s items once it's read, else null. */
    fun items(page: Int): List<T>? = pages[page]?.getOrNull()

    /** The item at [index], null while its page isn't read. */
    fun item(index: Int): T? = items(index / pageSize)?.getOrNull(index % pageSize)

    /** The letter of the item at [index]: the last to start at or before it. */
    fun letterAt(index: Int): String? = letters.lastOrNull { it.offset <= index }?.letter

    /** Where [letter] starts; a letter without items goes to the next that has some, or the last item. */
    fun letterOffset(letter: String): Int {
        for (l in RAIL_LETTERS.drop(RAIL_LETTERS.indexOf(letter).coerceAtLeast(0))) {
            letters.firstOrNull { it.letter == l }?.let { return it.offset }
        }
        return maxOf((total ?: 0) - 1, 0)
    }

    companion object {
        /** The letter rail's letters, `#` first. */
        val RAIL_LETTERS: List<String> = listOf("#") + ('A'..'Z').map { it.toString() }
    }
}
