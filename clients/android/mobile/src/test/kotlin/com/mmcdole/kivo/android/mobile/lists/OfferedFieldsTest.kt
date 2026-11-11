package com.mmcdole.kivo.android.mobile.lists

import com.mmcdole.kivo.api.models.WorkField
import com.mmcdole.kivo.api.models.WorkFieldOp
import com.mmcdole.kivo.api.models.WorkFieldType
import com.mmcdole.kivo.api.models.WorkFieldValueSource
import com.mmcdole.kivo.api.models.WorkKind
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class OfferedFieldsTest {
    private fun field(name: String, type: WorkFieldType = WorkFieldType.Choice, valueSource: WorkFieldValueSource? = WorkFieldValueSource.Inline) = WorkField(
        field = name,
        name = name,
        type = type,
        ops = listOf(WorkFieldOp("anyOf", -1)),
        kinds = listOf(WorkKind.Movie, WorkKind.Series),
        sortable = false,
        valueSource = valueSource,
    )

    private val fields = listOf(field("library"), field("genre"), field("year", WorkFieldType.Number, null))

    @Test
    fun libraryIsNeverOfferedAndAKeptFieldAlwaysIs() {
        val series = field("bookSeries").copy(kinds = listOf(WorkKind.Book))
        val all = fields + series
        assertFalse(offeredFields(all, listOf(WorkKind.Movie), null).any { it.field == "library" })
        assertFalse(offeredFields(all, listOf(WorkKind.Movie), null).any { it.field == "bookSeries" })
        assertTrue(offeredFields(all, listOf(WorkKind.Movie), "bookSeries").any { it.field == "bookSeries" })
    }

    @Test
    fun theSortsAreTheKindsOwnAndTheOneAlreadyChosen() {
        val rating = field("rating", WorkFieldType.Number, null).copy(sortable = true)
        val pages = field("pages", WorkFieldType.Number, null).copy(kinds = listOf(WorkKind.Book), sortable = true)
        val sorts = { kept: String? -> offeredFields(listOf(rating, pages), listOf(WorkKind.Movie), kept).filter { it.sortable }.map { it.field } }
        assertEquals(listOf("rating"), sorts(null))
        assertEquals(listOf("rating", "pages"), sorts("pages"))
    }
}
