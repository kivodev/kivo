package com.mmcdole.kivo.android.library

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.preferencesOf
import androidx.datastore.preferences.core.stringPreferencesKey
import com.mmcdole.kivo.core.data.GridQuery
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class GridMemoryTest {
    private fun titles(libraryId: String, query: GridQuery = GridQuery(libraryId, sort = "title")) = GridChoice(query)

    @Test
    fun eachLibrarysSortsAreTheOnesEveryAppOffers() {
        assertEquals(listOf("title", "author", "addedAt", "releaseDate"), GridValues.sortsOf("audiobooks"))
        assertEquals(listOf("title", "addedAt", "lastEpisodeAddedAt", "releaseDate", "rating", "runtime", "lastPlayedAt"), GridValues.sortsOf("shows"))
        assertEquals(listOf("title", "addedAt", "releaseDate", "rating", "runtime", "lastPlayedAt"), GridValues.sortsOf("movies"))
        assertEquals("runtime", decodeGrid("lib1", "sort=runtime").query.sort)
    }

    @Test
    fun aGridComesBackAsItWasKept() {
        val choice = GridChoice(
            GridQuery(
                "lib1",
                sort = "addedAt",
                played = "inProgress",
                resolutions = listOf("4k", "1080p"),
                dynamicRanges = listOf("dolbyVision"),
            ),
        )
        assertEquals(choice, decodeGrid("lib1", encodeGrid(choice)))
        val collections = titles("lib1").copy(section = Section.Collections)
        assertEquals(collections, decodeGrid("lib1", encodeGrid(collections)))
    }

    @Test
    fun theSamsungFiltersComeBackAsTheyWereKept() {
        val choice = GridChoice(
            GridQuery(
                "lib1",
                sort = "title",
                genres = listOf("Action & Adventure", "Sci-Fi, Fantasy"),
                yearFrom = 1990,
                yearTo = 1999,
                contentRatings = listOf("PG-13", "TV-MA"),
            ),
        )
        assertEquals(choice, decodeGrid("lib1", encodeGrid(choice)))
    }

    @Test
    fun theSamsungFiltersCountAsFiltersAndClear() {
        val query = GridQuery("lib1", sort = "title")
        listOf(
            query.copy(genres = listOf("Drama")),
            query.copy(yearFrom = 1990, yearTo = 1999),
            query.copy(contentRatings = listOf("PG")),
        ).forEach {
            assertTrue(it.filtered)
            assertEquals(query, it.unfiltered())
        }
    }

    @Test
    fun yearsGroupIntoDecadesNewestFirst() {
        assertEquals(listOf(2020, 2010, 1990), decadesOf(listOf(1994, 2011, 2019, 2024, 1999)))
    }

    @Test
    fun nothingKeptOpensOnTheTitlesByTitle() {
        assertEquals(titles("lib1"), decodeGrid("lib1", ""))
    }

    @Test
    fun whatThisVersionDoesntKnowIsLeftOut() {
        assertEquals(
            titles("lib1", GridQuery("lib1", sort = "title", resolutions = listOf("sd"))),
            decodeGrid("lib1", "kind=collection&section=bogus&sort=bogus&played=maybe&resolution=8k,sd&view=list"),
        )
    }

    // Migration (api-naming): remove after every client has updated past this release.
    @Test
    fun aGridSavedByAnEarlierVersionReadsAsItWas() {
        assertEquals(
            titles("lib1", GridQuery("lib1", sort = "lastEpisodeAddedAt", dynamicRanges = listOf("hdr"))),
            decodeGrid("lib1", "sort=lastEpisodeAdded&hdr=hdr"),
        )
    }

    @Test
    fun clearingTheFiltersKeepsTheSort() {
        val query = GridQuery("lib1", sort = "rating", played = "played")
        assertTrue(query.filtered)
        val clear = query.unfiltered()
        assertFalse(clear.filtered)
        assertEquals("rating", clear.sort)
    }

    @Test
    fun everyLibrarysSavedGridIsReadAtOnce() {
        val store = object : DataStore<Preferences> {
            override val data = MutableStateFlow(
                preferencesOf(
                    stringPreferencesKey("grid.lib1") to "sort=addedAt&played=unplayed",
                    stringPreferencesKey("grid.lib2") to "section=collections",
                    stringPreferencesKey("server.last") to "abc",
                ),
            )

            override suspend fun updateData(transform: suspend (t: Preferences) -> Preferences) =
                transform(data.value).also { data.value = it }
        }
        val saved = runBlocking { SavedGrids(store).all.first() }
        assertEquals(
            mapOf(
                "lib1" to titles("lib1", GridQuery("lib1", sort = "addedAt", played = "unplayed")),
                "lib2" to titles("lib2").copy(section = Section.Collections),
            ),
            saved,
        )
        assertEquals(saved["lib1"], gridChoice(saved, "lib1"))
        assertEquals(titles("lib3"), gridChoice(saved, "lib3"))
    }
}
