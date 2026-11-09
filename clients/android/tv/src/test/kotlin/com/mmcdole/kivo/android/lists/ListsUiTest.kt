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
    fun myListComesApartFromTheListsMade() {
        val myList = WorkList("my", "My List", "manual", builtIn = true, workCount = 2)
        val made = WorkList("d", "Date Night", "manual", builtIn = false, workCount = 1)
        val ui = listsUi(
            listOf(myList, made, WorkList("x", "Later", "future", builtIn = false, workCount = 1)),
            WorkSummaryPage(listOf(work("m1", "movie")), total = 1),
        )
        assertEquals(myList, ui.myList)
        assertEquals(listOf("m1"), ui.works.items.map { it.id })
        assertEquals(listOf(made), ui.lists)
    }

    @Test
    fun smartListsComeAfterTheManualOnes() {
        val smart = WorkList("s", "90s comedies", "smart", builtIn = false, workCount = 3)
        val made = WorkList("d", "Date Night", "manual", builtIn = false, workCount = 1)
        val ui = listsUi(listOf(smart, made), WorkSummaryPage(emptyList(), total = 0))
        assertEquals(listOf(made, smart), ui.lists)
    }
}
