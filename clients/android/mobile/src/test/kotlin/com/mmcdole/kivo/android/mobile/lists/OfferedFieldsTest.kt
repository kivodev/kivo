package com.mmcdole.kivo.android.mobile.lists

import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkFieldOp
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class OfferedFieldsTest {
    private fun field(name: String, type: String = "choice", valueSource: String? = "inline") = WorkField(
        field = name,
        name = name,
        type = type,
        ops = listOf(WorkFieldOp("anyOf", -1)),
        kinds = listOf("movie", "series"),
        sortable = false,
        valueSource = valueSource,
    )

    private val fields = listOf(field("library"), field("genre"), field("year", "number", null))

    @Test
    fun libraryIsNeverOfferedAndAKeptFieldAlwaysIs() {
        val series = field("bookSeries").copy(kinds = listOf("book"))
        val all = fields + series
        assertFalse(offeredFields(all, listOf("movie"), null).any { it.field == "library" })
        assertFalse(offeredFields(all, listOf("movie"), null).any { it.field == "bookSeries" })
        assertTrue(offeredFields(all, listOf("movie"), "bookSeries").any { it.field == "bookSeries" })
    }

    @Test
    fun theSortsAreTheKindsOwnAndTheOneAlreadyChosen() {
        val rating = field("rating", "number", null).copy(sortable = true)
        val pages = field("pages", "number", null).copy(kinds = listOf("book"), sortable = true)
        val sorts = { kept: String? -> offeredFields(listOf(rating, pages), listOf("movie"), kept).filter { it.sortable }.map { it.field } }
        assertEquals(listOf("rating"), sorts(null))
        assertEquals(listOf("rating", "pages"), sorts("pages"))
    }
}
