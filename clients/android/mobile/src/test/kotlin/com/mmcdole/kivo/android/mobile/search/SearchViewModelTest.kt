package com.mmcdole.kivo.android.mobile.search

import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.android.mobile.work
import com.mmcdole.kivo.api.models.SearchResults
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class SearchViewModelTest : CoreTest() {
    init {
        server.get("/api/v1/search", SearchResults.serializer()) { SearchResults(listOf(work("m1", "Charade")), emptyList(), emptyList()) }
    }

    @Test
    fun searchesOnceTypingSettlesForTheTrimmedQuery() {
        val model = SearchViewModel(signIn())
        model.setQuery("ch")
        model.setQuery("cha ")
        val state = awaitValue(model.state) { it.answer?.results != null && !it.searching }
        assertEquals("cha", state.answer!!.query)
        assertEquals("Charade", state.answer!!.results!!.works.single().title)
        assertEquals(1, server.seen.count { it.startsWith("GET /api/v1/search") })
    }

    @Test
    fun clearingTheFieldClearsTheResults() {
        val model = SearchViewModel(signIn(), initial = "cha")
        awaitValue(model.state) { it.answer != null }
        model.setQuery("")
        assertNull(awaitValue(model.state) { it.query.isEmpty() && it.answer == null }.answer)
    }
}
