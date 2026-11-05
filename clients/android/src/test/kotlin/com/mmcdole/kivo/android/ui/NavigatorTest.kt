package com.mmcdole.kivo.android.ui

import com.mmcdole.kivo.android.settings.SettingsGroup
import kotlin.test.Test
import kotlin.test.assertEquals

class NavigatorTest {
    @Test
    fun backAtHomeAsksToLeave() {
        assertEquals(Back.Exit, Navigator().back)
    }

    @Test
    fun backFromADestinationOpensTheRailAndTheRailsBackGoesHome() {
        val navigator = Navigator()
        for (destination in listOf(Screen.Search, Screen.Settings, Screen.Favorites, Screen.Library("movies"))) {
            navigator.open(destination)
            assertEquals(Back.Rail, navigator.back, "$destination")
            navigator.open(Screen.Home)
            assertEquals(listOf<Screen>(Screen.Home), navigator.stack.map { it.screen })
            assertEquals(Back.Exit, navigator.back)
        }
    }

    @Test
    fun aSettingsPageGoesBackToSettingsThenTheRail() {
        val navigator = Navigator()
        navigator.open(Screen.Settings)
        navigator.push(Screen.SettingsPage(SettingsGroup.Account))
        assertEquals(Back.Pop, navigator.back)

        navigator.pop()
        assertEquals(Screen.Settings, navigator.current.screen)
        assertEquals(Back.Rail, navigator.back)
    }

    @Test
    fun favoritesOpenedFromHomeIsADestinationToo() {
        val navigator = Navigator()
        navigator.push(Screen.Favorites)
        assertEquals(Back.Rail, navigator.back)

        navigator.push(Screen.FavoritesGrid("movie"))
        assertEquals(Back.Pop, navigator.back)
    }

    @Test
    fun theRailMarksTheDestinationAPageSitsOn() {
        val navigator = Navigator()
        navigator.push(Screen.Work("w1"))
        assertEquals(Screen.Home, navigator.destination)

        navigator.open(Screen.Search)
        navigator.push(Screen.Person("p1"))
        assertEquals(Screen.Search, navigator.destination)

        navigator.open(Screen.Settings)
        navigator.push(Screen.SettingsPage(SettingsGroup.About))
        assertEquals(Screen.Settings, navigator.destination)

        // About's Credits, a page further down.
        navigator.push(Screen.Credits)
        assertEquals(Screen.Settings, navigator.destination)
        assertEquals(Back.Pop, navigator.back)
    }

    @Test
    fun pagesAboveGoBackAScreen() {
        val navigator = Navigator()
        // A Watch Next program's page over Home.
        navigator.push(Screen.Work("w1"))
        assertEquals(Back.Pop, navigator.back)

        // A detail page's Go to library opens it as a destination.
        navigator.open(Screen.Library("movies"))
        navigator.push(Screen.Work("w2"))
        navigator.push(Screen.Library("shows"))
        assertEquals(Back.Pop, navigator.back)
        navigator.pop()
        navigator.pop()
        assertEquals(Back.Rail, navigator.back)
    }
}
