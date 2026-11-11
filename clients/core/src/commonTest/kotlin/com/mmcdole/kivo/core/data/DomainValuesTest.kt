package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.Credit
import com.mmcdole.kivo.api.models.PersonSummary
import com.mmcdole.kivo.api.models.Resolution
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class DomainValuesTest {
    /** The spellings a device has saved its grids with, and a work query is written with. */
    @Test
    fun aGridsValuesKeepTheirWireSpelling() {
        assertEquals(
            listOf("title", "addedAt", "lastEpisodeAddedAt", "releaseDate", "rating", "runtimeMs", "lastPlayedAt", "author"),
            GridSort.entries.map { it.value },
        )
        assertEquals(listOf("unplayed", "inProgress", "played"), WorkPlayState.entries.map { it.value })
        assertEquals(listOf("hdr", "dolbyVision"), DynamicRange.entries.map { it.value })
        assertEquals(listOf("4k", "1080p", "720p", "sd", ""), Resolution.entries.map { it.value })
    }

    @Test
    fun aStoredValueReadsBackAsItsCaseAndOneThisVersionDoesntKnowAsNone() {
        for (sort in GridSort.entries) assertEquals(sort, GridSort.of(sort.value))
        for (state in WorkPlayState.entries) assertEquals(state, WorkPlayState.of(state.value))
        for (range in DynamicRange.entries) assertEquals(range, DynamicRange.of(range.value))
        assertNull(GridSort.of("lastEpisodeAdded"), "the spelling before the rename isn't a sort: its migration renames it first")
        assertNull(GridSort.of(null))
        assertNull(WorkPlayState.of("skipped"))
        assertNull(DynamicRange.of("hdr10000"))
        assertEquals(Resolution.Unknown, Resolution.of("8k"))
    }

    @Test
    fun aCreditsRoleIsKnownByItsNameAndAnyOtherKeepsItsText() {
        assertEquals(CreditRole.Director, Credit("p1", "Denis Villeneuve", "director").knownRole)
        assertEquals(CreditRole.Narrator, PersonSummary("p2", "Ray Porter", "narrator", 3).knownRole)
        val producer = Credit("p3", "Mary Parent", "producer")
        assertEquals(CreditRole.Unknown, producer.knownRole)
        assertEquals("producer", producer.role)
        for (role in CreditRole.entries) assertEquals(role, CreditRole.of(role.value))
    }
}
