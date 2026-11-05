package com.mmcdole.kivo.android.settings

import com.mmcdole.kivo.android.main.tvLibraries
import com.mmcdole.kivo.api.models.Library
import kotlin.test.Test
import kotlin.test.assertEquals

private fun library(id: String, kind: String = "movies") = Library(id, id, pinned = true, kind = kind, workCount = 0)

class SettingsTest {
    @Test
    fun groupsAreInSettingsOrder() {
        assertEquals(
            listOf("Account", "Playback", "Audio", "Subtitles", "Quality", "Display", "Libraries", "About"),
            SettingsGroup.entries.map { it.name },
        )
    }

    @Test
    fun movingALibraryPassesOverTheOnesTheTvDoesntList() {
        val all = listOf(library("movies"), library("books", "audiobooks"), library("shows", "shows"), library("home", "videos"))

        val up = moved(all, "shows", -1)
        assertEquals(listOf("shows", "books", "movies", "home"), up.map { it.id })
        assertEquals(listOf("shows", "movies", "home"), tvLibraries(up).map { it.id })

        assertEquals(listOf("movies", "books", "home", "shows"), moved(all, "shows", 1).map { it.id })
    }

    @Test
    fun movingPastEitherEndChangesNothing() {
        val all = listOf(library("books", "audiobooks"), library("movies"), library("shows", "shows"))
        assertEquals(all, moved(all, "movies", -1))
        assertEquals(all, moved(all, "shows", 1))
    }
}
