package com.mmcdole.kivo.android.home

import com.mmcdole.kivo.android.ui.CardShape
import com.mmcdole.kivo.api.models.HomeRow
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryRef
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkSummary
import kotlin.test.Test
import kotlin.test.assertEquals

class ShownRowsTest {
    @Test
    fun aListRowDropsTheBooksTheTvDoesntShow() {
        val book = work.copy(id = "b", kind = "book")
        val shown = shownRows(listOf(row("list:1", "list", items = listOf(work, book))), emptyList())
        assertEquals(listOf("w"), shown.single().row.items.map { it.id })
        assertEquals(emptyList(), shownRows(listOf(row("list:2", "list", items = listOf(book))), emptyList()))
    }

    private val movies = Library("movies", "Movies", true, "movies", 10)
    private val clips = Library("clips", "Clips", true, "videos", 10)
    private val work = WorkSummary(id = "w", kind = "movie", title = "W", matchState = "matched", available = true, images = WorkImages(), libraryId = "movies")

    private fun row(id: String, kind: String, library: Library? = null, items: List<WorkSummary> = listOf(work)) =
        HomeRow(id, kind, items, library?.let { LibraryRef(it.id, it.name, it.kind) })

    @Test
    fun eachRowComesWithItsLibraryAndShape() {
        val rows = shownRows(
            listOf(row("cw", "continueWatching"), row("ra:movies", "recentlyAdded", movies), row("ra:clips", "recentlyAdded", clips), row("list:1", "list")),
            listOf(movies, clips),
        )
        assertEquals(
            listOf(
                ShownRow(rows[0].row, null, CardShape.Landscape),
                ShownRow(rows[1].row, movies, CardShape.Poster),
                ShownRow(rows[2].row, clips, CardShape.Landscape),
                ShownRow(rows[3].row, null, CardShape.Poster),
            ),
            rows,
        )
    }

    @Test
    fun emptyRowsUnknownKindsAndLibrariesNotShownAreLeftOut() {
        val books = Library("books", "Books", true, "audiobooks", 10)
        val rows = shownRows(
            listOf(
                row("cw", "continueWatching", items = emptyList()),
                row("cl", "continueListening"),
                row("ra:books", "recentlyAdded", books),
                row("ra:movies", "recentlyAdded", movies),
            ),
            listOf(movies),
        )
        assertEquals(listOf("ra:movies"), rows.map { it.row.id })
    }
}
