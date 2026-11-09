package com.mmcdole.kivo.android.mobile.lists

import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.android.mobile.refuse
import com.mmcdole.kivo.android.mobile.reply
import com.mmcdole.kivo.android.mobile.work
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkList
import com.mmcdole.kivo.api.models.WorkListList
import com.mmcdole.kivo.api.models.WorkSummaryPage
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ListsViewModelsTest : CoreTest() {
    private val myList = WorkList("my", "My List", "manual", builtIn = true, workCount = 2)
    private val dateNight = WorkList("d", "Date Night", "manual", builtIn = false, workCount = 1)
    private var lists = listOf(myList, dateNight, WorkList("x", "Later", "smart", builtIn = false, workCount = 9))
    private var held = listOf("my")

    init {
        server.get("/api/v1/lists", WorkListList.serializer()) { WorkListList(lists) }
        server.get("/api/v1/lists/my/works", WorkSummaryPage.serializer()) { WorkSummaryPage(listOf(work("m1"), work("m2")), total = 2) }
        server.get("/api/v1/works/m1", WorkDetail.serializer()) {
            WorkDetail(
                id = "m1", kind = "movie", title = "m1", matchState = "matched", available = true, images = WorkImages(),
                genres = emptyList(), studios = emptyList(), credits = emptyList(), externalIds = emptyList(), versions = emptyList(),
                extras = emptyList(), collections = emptyList(), listIds = held,
            )
        }
        server.on("POST", "/api/v1/lists/d/works") { respond("", HttpStatusCode.NoContent) }
        server.on("DELETE", "/api/v1/lists/my/works/m1") { respond("", HttpStatusCode.NoContent) }
        server.on("POST", "/api/v1/lists") {
            reply(WorkList.serializer(), WorkList("n", "Road Trip", "manual", builtIn = false, workCount = 0), HttpStatusCode.Created)
        }
        server.on("POST", "/api/v1/lists/n/works") { respond("", HttpStatusCode.NoContent) }
        server.on("PATCH", "/api/v1/lists/d") { refuse(HttpStatusCode.Conflict, "nameTaken") }
    }

    @Test
    fun theTabShowsMyListsCoversAndTheManualListsMade() {
        val model = ListsViewModel(signIn())
        val state = awaitValue(model.state) { it.lists != null && it.covers.isNotEmpty() }
        assertEquals(myList, state.myList)
        assertEquals(listOf("m1", "m2"), state.covers.map { it.id })
        assertEquals(listOf(dateNight), state.manual)
    }

    @Test
    fun anEmptyMyListIsLeftOut() {
        lists = listOf(myList.copy(workCount = 0), dateNight)
        val model = ListsViewModel(signIn())
        val state = awaitValue(model.state) { it.lists != null }
        assertNull(state.myList)
        assertEquals(listOf(dateNight), state.manual)
    }

    @Test
    fun theChooserTicksTheListsHoldingTheWorkAndAddsOrRemovesAsTapped() {
        val model = AddToListViewModel(signIn(), "m1")
        val choices = awaitValue(model.choices) { it != null }!!
        assertEquals(listOf("my" to true, "d" to false), choices.map { it.list.id to it.holding })
        model.toggle(choices.first { it.list.id == "my" })
        model.toggle(choices.first { it.list.id == "d" })
        runBlocking {
            withTimeout(5_000) { while ("DELETE /api/v1/lists/my/works/m1" !in server.seen || "POST /api/v1/lists/d/works" !in server.seen) delay(10) }
        }
    }

    @Test
    fun newListMakesTheListWithTheWorkOnIt() {
        val model = AddToListViewModel(signIn(), "m1")
        assertNull(runBlocking { model.create("Road Trip") })
        assertTrue("POST /api/v1/lists" in server.seen)
        assertTrue("POST /api/v1/lists/n/works" in server.seen)
    }

    @Test
    fun aNameInUseIsSaidSo() {
        val model = ListViewModel(signIn(), "d")
        assertEquals("nameTaken", runBlocking { model.rename("Date Night") }?.code)
    }
}
