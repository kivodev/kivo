package com.mmcdole.kivo.android.mobile.home

import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.android.mobile.refuse
import com.mmcdole.kivo.android.mobile.work
import com.mmcdole.kivo.api.models.Home
import com.mmcdole.kivo.api.models.HomeRow
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryList
import com.mmcdole.kivo.api.models.LibraryRef
import com.mmcdole.kivo.core.RequestProblem
import io.ktor.http.HttpStatusCode
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class HomeViewModelTest : CoreTest() {
    private val movies = Library("lib1", "Movies", pinned = true, kind = "movies", workCount = 2)

    private var rows = listOf(
        HomeRow("continueWatching", "continueWatching", listOf(work("m1"))),
        HomeRow("someday", "aKindFromLater", listOf(work("m2"))),
        HomeRow("favorites", "favorites", emptyList()),
        HomeRow("recentlyAdded:lib1", "recentlyAdded", listOf(work("m2"), work("m1")), LibraryRef("lib1", "Movies", "movies")),
    )

    init {
        server.get("/api/v1/home", Home.serializer()) { Home(rows) }
        server.get("/api/v1/libraries", LibraryList.serializer()) { LibraryList(listOf(movies)) }
    }

    @Test
    fun showsTheRowsItKnowsInTheServersOrderWithTheirLibraries() {
        val model = HomeViewModel(signIn())
        val state = awaitValue(model.state) { it.rows != null && it.rows!!.first().library == null && it.rows!!.last().library != null }
        assertEquals(listOf("continueWatching", "recentlyAdded:lib1"), state.rows!!.map { it.id })
        assertEquals(movies, state.rows!!.last().library)
        assertEquals("Den", state.serverName)
        assertEquals("A", state.initial)
        assertNull(state.problem)
    }

    @Test
    fun aPullReadsHomeAgainAndSaysWhenItsDone() {
        val model = HomeViewModel(signIn())
        awaitValue(model.state) { it.rows?.size == 2 }
        rows = rows.filter { it.kind != "continueWatching" }
        model.refresh()
        val state = awaitValue(model.state) { it.rows?.size == 1 && !it.refreshing }
        assertEquals("recentlyAdded:lib1", state.rows!!.single().id)
    }

    @Test
    fun aRefusalIsTheProblemWhileThereIsNothingToShow() {
        server.on("GET", "/api/v1/home") { refuse(HttpStatusCode.Forbidden, "forbidden") }
        val model = HomeViewModel(signIn())
        val state = awaitValue(model.state) { it.problem != null }
        assertEquals("forbidden", (state.problem as RequestProblem.Refused).code)
        assertNull(state.rows)
    }
}
