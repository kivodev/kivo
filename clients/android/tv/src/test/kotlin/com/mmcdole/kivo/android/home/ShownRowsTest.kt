package com.mmcdole.kivo.android.home

import com.mmcdole.kivo.android.ui.CardShape
import com.mmcdole.kivo.api.models.HomeRow
import com.mmcdole.kivo.api.models.HomeRowKind
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.LibraryRef
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkSummary
import kotlin.test.Test
import kotlin.test.assertEquals

class ShownRowsTest {
    @Test
    fun aListRowDropsTheBooksTheTvDoesntShow() {
        val book = work.copy(id = "b", kind = WorkKind.Book)
        val shown = shownRows(listOf(row("list:1", HomeRowKind.List, items = listOf(work, book))), emptyList())
        assertEquals(listOf("w"), shown.single().row.items.map { it.id })
        assertEquals(emptyList(), shownRows(listOf(row("list:2", HomeRowKind.List, items = listOf(book))), emptyList()))
    }

    private val movies = Library("movies", "Movies", true, LibraryKind.Movies, 10)
    private val clips = Library("clips", "Clips", true, LibraryKind.Videos, 10)
    private val work = WorkSummary(id = "w", kind = WorkKind.Movie, title = "W", matchState = MatchState.Matched, available = true, images = WorkImages(), libraryId = "movies")

    private fun row(id: String, kind: HomeRowKind, library: Library? = null, items: List<WorkSummary> = listOf(work)) =
        HomeRow(id, kind, items, library?.let { LibraryRef(it.id, it.name, it.kind) })

    @Test
    fun eachRowComesWithItsLibraryAndShape() {
        val rows = shownRows(
            listOf(row("cw", HomeRowKind.ContinueWatching), row("ra:movies", HomeRowKind.RecentlyAdded, movies), row("ra:clips", HomeRowKind.RecentlyAdded, clips), row("list:1", HomeRowKind.List)),
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
        val books = Library("books", "Books", true, LibraryKind.Audiobooks, 10)
        val rows = shownRows(
            listOf(
                row("cw", HomeRowKind.ContinueWatching, items = emptyList()),
                row("cl", HomeRowKind.ContinueListening),
                row("ra:books", HomeRowKind.RecentlyAdded, books),
                row("ra:movies", HomeRowKind.RecentlyAdded, movies),
            ),
            listOf(movies),
        )
        assertEquals(listOf("ra:movies"), rows.map { it.row.id })
    }
}
