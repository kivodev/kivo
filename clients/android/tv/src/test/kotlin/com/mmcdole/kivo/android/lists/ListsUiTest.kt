package com.mmcdole.kivo.android.lists

import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.api.models.WorkSummaryPage
import kotlin.test.Test
import kotlin.test.assertEquals

class ListsUiTest {
    private fun work(id: String, kind: String) =
        WorkSummary(id = id, kind = kind, title = id, matchState = "matched", available = true, images = WorkImages())

    @Test
    fun myListComesApartFromTheListsMadeAndBooksAreLeftOut() {
        val myList = WorkList("my", "My List", "manual", builtIn = true, workCount = 2)
        val made = WorkList("d", "Date Night", "manual", builtIn = false, workCount = 1)
        val ui = listsUi(
            listOf(myList, made, WorkList("x", "Later", "smart", builtIn = false, workCount = 1)),
            WorkSummaryPage(listOf(work("m1", "movie"), work("b1", "book")), total = 2),
        )
        assertEquals(myList, ui.myList)
        assertEquals(listOf("m1"), ui.works.items.map { it.id })
        assertEquals(listOf(made), ui.lists)
    }
}
