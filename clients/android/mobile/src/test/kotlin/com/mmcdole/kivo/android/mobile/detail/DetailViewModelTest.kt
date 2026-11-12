package com.mmcdole.kivo.android.mobile.detail

import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.android.mobile.work
import com.mmcdole.kivo.api.models.SearchResults
import com.mmcdole.kivo.core.ProblemCode
import kotlinx.coroutines.runBlocking
import kotlin.test.Test
import kotlin.test.assertEquals

class DetailViewModelTest : CoreTest() {
    init {
        server.get("/api/v1/search", SearchResults.serializer()) { SearchResults(listOf(work("m1", "Charade")), emptyList(), emptyList()) }
    }

    @Test
    fun aWorkTheServerNoLongerHasFailsThePageThoughItsCardIsKnown() {
        val session = signIn()
        runBlocking { session.library.search("cha", null) }
        val model = held { DetailViewModel(session, "m1") }
        val state = awaitValue(model.state) { it.page.problem != null }
        assertEquals("Charade", state.page.head?.title)
        assertEquals(ProblemCode.NotFound, state.failure?.knownCode)
    }
}
