package com.mmcdole.kivo.android.library

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.preferencesOf
import androidx.datastore.preferences.core.stringPreferencesKey
import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.Resolution
import com.mmcdole.kivo.core.data.DynamicRange
import com.mmcdole.kivo.core.data.GridQuery
import com.mmcdole.kivo.core.data.GridSort
import com.mmcdole.kivo.core.data.WorkPlayState
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class GridMemoryTest {
    private fun titles(libraryId: String, query: GridQuery = GridQuery(libraryId, sort = GridSort.Title)) = GridChoice(query)

    @Test
    fun eachLibrarysSortsAreTheOnesEveryAppOffers() {
        assertEquals(listOf(GridSort.Title, GridSort.Author, GridSort.AddedAt, GridSort.ReleaseDate), sortsOf(LibraryKind.Audiobooks))
        assertEquals(
            listOf(GridSort.Title, GridSort.AddedAt, GridSort.LastEpisodeAddedAt, GridSort.ReleaseDate, GridSort.Rating, GridSort.Runtime, GridSort.LastPlayedAt),
            sortsOf(LibraryKind.Shows),
        )
        assertEquals(
            listOf(GridSort.Title, GridSort.AddedAt, GridSort.ReleaseDate, GridSort.Rating, GridSort.Runtime, GridSort.LastPlayedAt),
            sortsOf(LibraryKind.Movies),
        )
        assertEquals(GridSort.Runtime, decodeGrid("lib1", "sort=runtimeMs").query.sort)
        assertEquals(GridSort.Title, decodeGrid("lib1", "sort=runtime").query.sort, "a sort this app doesn't offer")
    }

    @Test
    fun aGridComesBackAsItWasKept() {
        val choice = GridChoice(
            GridQuery(
                "lib1",
                sort = GridSort.AddedAt,
                played = WorkPlayState.InProgress,
                resolutions = listOf(Resolution.FourK, Resolution.P1080),
                dynamicRanges = listOf(DynamicRange.DolbyVision),
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
                sort = GridSort.Title,
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
        val query = GridQuery("lib1", sort = GridSort.Title)
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
            titles("lib1", GridQuery("lib1", sort = GridSort.Title, resolutions = listOf(Resolution.Sd))),
            decodeGrid("lib1", "kind=collection&section=bogus&sort=bogus&played=maybe&resolution=8k,sd&view=list"),
        )
    }

    // Migration (api-naming): remove after every client has updated past this release.
    @Test
    fun aGridSavedByAnEarlierVersionReadsAsItWas() {
        assertEquals(
            titles("lib1", GridQuery("lib1", sort = GridSort.LastEpisodeAddedAt, dynamicRanges = listOf(DynamicRange.Hdr))),
            decodeGrid("lib1", "sort=lastEpisodeAdded&hdr=hdr"),
        )
    }

    // Migration (api-naming): remove after every client has updated past this release.
    @Test
    fun everyPartOfAGridSavedByAnEarlierVersionIsKeptAndSavedAgainAsNow() {
        val stored = "section=collections&sort=lastEpisodeAdded&played=inProgress&decade=1990&genre=Action+%26+Adventure,Drama" +
            "&contentRating=PG-13&resolution=4k,720p&hdr=hdr,dolbyVision"
        val choice = decodeGrid("lib1", stored)
        assertEquals(
            GridChoice(
                GridQuery(
                    "lib1",
                    sort = GridSort.LastEpisodeAddedAt,
                    played = WorkPlayState.InProgress,
                    genres = listOf("Action & Adventure", "Drama"),
                    yearFrom = 1990,
                    yearTo = 1999,
                    contentRatings = listOf("PG-13"),
                    resolutions = listOf(Resolution.FourK, Resolution.P720),
                    dynamicRanges = listOf(DynamicRange.Hdr, DynamicRange.DolbyVision),
                ),
                Section.Collections,
            ),
            choice,
        )
        val now = "section=collections&sort=lastEpisodeAddedAt&played=inProgress&decade=1990&genre=Action+%26+Adventure,Drama" +
            "&contentRating=PG-13&resolution=4k,720p&dynamicRange=hdr,dolbyVision"
        assertEquals(now, encodeGrid(choice))
        assertEquals(now, encodeGrid(decodeGrid("lib1", now)))
    }

    @Test
    fun aSortOrResolutionThisVersionDoesntKnowIsDropped() {
        val choice = decodeGrid("lib1", "sort=bogus&played=unplayed&resolution=8k,1080p&dynamicRange=hdr10plus")
        assertEquals(titles("lib1", GridQuery("lib1", sort = GridSort.Title, played = WorkPlayState.Unplayed, resolutions = listOf(Resolution.P1080))), choice)
        assertEquals("sort=title&played=unplayed&resolution=1080p", encodeGrid(choice))
    }

    @Test
    fun clearingTheFiltersKeepsTheSort() {
        val query = GridQuery("lib1", sort = GridSort.Rating, played = WorkPlayState.Played)
        assertTrue(query.filtered)
        val clear = query.unfiltered()
        assertFalse(clear.filtered)
        assertEquals(GridSort.Rating, clear.sort)
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
                "lib1" to titles("lib1", GridQuery("lib1", sort = GridSort.AddedAt, played = WorkPlayState.Unplayed)),
                "lib2" to titles("lib2").copy(section = Section.Collections),
            ),
            saved,
        )
        assertEquals(saved["lib1"], gridChoice(saved, "lib1"))
        assertEquals(titles("lib3"), gridChoice(saved, "lib3"))
    }
}
