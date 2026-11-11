package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.ExtraInfo
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkSummary
import kotlin.test.Test
import kotlin.test.assertEquals

class TrailersTest {
    private fun extra(id: String, type: String?) =
        WorkSummary(id, WorkKind.Extra, id, MatchState.Matched, true, WorkImages(), extra = type?.let { ExtraInfo(it) })

    private fun work(extras: List<WorkSummary>) = WorkDetail(
        "w1", WorkKind.Movie, "Dune", MatchState.Matched, true, WorkImages(), emptyList(), emptyList(), emptyList(), emptyList(), emptyList(), extras, emptyList(),
        libraryId = "l1",
    )

    @Test
    fun aWorksTrailerFilesAreApartFromItsOtherExtrasEachInOrder() {
        val work = work(listOf(extra("making", "featurette"), extra("teaser", "trailer"), extra("untyped", null), extra("final", "trailer")))
        assertEquals(listOf("teaser", "final"), work.localTrailers().map { it.id })
        assertEquals(listOf("making", "untyped"), work.otherExtras().map { it.id })
    }

    @Test
    fun aWorkWithoutExtrasHasNeither() {
        assertEquals(emptyList(), work(emptyList()).localTrailers())
        assertEquals(emptyList(), work(emptyList()).otherExtras())
    }
}
