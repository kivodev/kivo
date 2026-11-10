package com.mmcdole.kivo.android.lists

import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.api.models.FieldError
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkListSmart
import com.mmcdole.kivo.api.models.WorkQuery
import com.mmcdole.kivo.api.models.WorkSummary
import kotlin.test.Test
import kotlin.test.assertEquals

class ListsUiTest {
    private fun work(id: String, kind: String) =
        WorkSummary(id = id, kind = kind, title = id, matchState = "matched", available = true, images = WorkImages())

    @Test
    fun everyListIsAShelfInItsOrderWithoutBooks() {
        val myList = WorkList("my", "Watchlist", "manual", builtIn = true, workCount = 2, items = listOf(work("m1", "movie"), work("b1", "book")))
        val smart = WorkList("s", "90s comedies", "smart", builtIn = false, workCount = 3, items = listOf(work("m2", "movie")))
        val made = WorkList("d", "Date Night", "manual", builtIn = false, workCount = 0, items = emptyList())
        val later = WorkList("x", "Later", "future", builtIn = false, workCount = 1)
        assertEquals(
            listOf(Shelf(myList, listOf(work("m1", "movie"))), Shelf(smart, smart.items.orEmpty()), Shelf(made, emptyList())),
            shelves(listOf(myList, smart, made, later)),
        )
    }

    @Test
    fun watchlistHasNoShelfWithNothingToShow() {
        val myList = WorkList("my", "Watchlist", "manual", builtIn = true, workCount = 1, items = listOf(work("b1", "book")))
        assertEquals(emptyList(), shelves(listOf(myList)))
    }

    @Test
    fun anEmptyListSaysWhy() {
        val manual = WorkList("d", "Date Night", "manual", builtIn = false, workCount = 0)
        val rules = WorkListSmart(WorkQuery(listOf("movie")))
        val smart = WorkList("s", "90s comedies", "smart", builtIn = false, workCount = 0, smart = rules)
        val broken = smart.copy(smart = rules.copy(errors = listOf(FieldError("genre", "unknownField"))))
        assertEquals(R.string.list_empty, emptyListMessage(manual))
        assertEquals(R.string.list_nothing_matches, emptyListMessage(smart))
        assertEquals(R.string.list_rules_broken, emptyListMessage(broken))
    }
}
