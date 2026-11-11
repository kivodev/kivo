package com.mmcdole.kivo.android.lists

import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.api.models.FieldError
import com.mmcdole.kivo.api.models.ListKind
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkListSmart
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.api.models.WorkSummary
import kotlin.test.Test
import kotlin.test.assertEquals

class ListsUiTest {
    private fun work(id: String, kind: WorkKind) =
        WorkSummary(id = id, kind = kind, title = id, matchState = MatchState.Matched, available = true, images = WorkImages())

    @Test
    fun everyListIsAShelfInItsOrderWithoutBooks() {
        val myList = WorkList("my", "Watchlist", ListKind.Manual, builtIn = true, workCount = 2, items = listOf(work("m1", WorkKind.Movie), work("b1", WorkKind.Book)))
        val smart = WorkList("s", "90s comedies", ListKind.Smart, builtIn = false, workCount = 3, items = listOf(work("m2", WorkKind.Movie)))
        val made = WorkList("d", "Date Night", ListKind.Manual, builtIn = false, workCount = 0, items = emptyList())
        val later = WorkList("x", "Later", ListKind.Unknown, builtIn = false, workCount = 1)
        assertEquals(
            listOf(Shelf(myList, listOf(work("m1", WorkKind.Movie))), Shelf(smart, smart.items.orEmpty()), Shelf(made, emptyList())),
            shelves(listOf(myList, smart, made, later)),
        )
    }

    @Test
    fun watchlistHasNoShelfWithNothingToShow() {
        val myList = WorkList("my", "Watchlist", ListKind.Manual, builtIn = true, workCount = 1, items = listOf(work("b1", WorkKind.Book)))
        assertEquals(emptyList(), shelves(listOf(myList)))
    }

    @Test
    fun anEmptyListSaysWhy() {
        val manual = WorkList("d", "Date Night", ListKind.Manual, builtIn = false, workCount = 0)
        val rules = WorkListSmart(WorkQuery(listOf(WorkKind.Movie)))
        val smart = WorkList("s", "90s comedies", ListKind.Smart, builtIn = false, workCount = 0, smart = rules)
        val broken = smart.copy(smart = rules.copy(errors = listOf(FieldError("genre", "unknownField"))))
        assertEquals(R.string.list_empty, emptyListMessage(manual))
        assertEquals(R.string.list_nothing_matches, emptyListMessage(smart))
        assertEquals(R.string.list_rules_broken, emptyListMessage(broken))
    }
}
