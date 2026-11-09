package com.mmcdole.kivo.android.ui

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
        for (destination in listOf(Screen.Search, Screen.Settings, Screen.Lists, Screen.Library("movies"))) {
            navigator.open(destination)
            assertEquals(Back.Rail, navigator.back, "$destination")
            navigator.open(Screen.Home)
            assertEquals(listOf<Screen>(Screen.Home), navigator.stack.map { it.screen })
            assertEquals(Back.Exit, navigator.back)
        }
    }

    @Test
    fun creditsGoBackToSettingsThenTheRail() {
        val navigator = Navigator()
        navigator.open(Screen.Settings)
        navigator.push(Screen.Credits)
        assertEquals(Back.Pop, navigator.back)

        navigator.pop()
        assertEquals(Screen.Settings, navigator.current.screen)
        assertEquals(Back.Rail, navigator.back)
    }

    @Test
    fun listsOpenedFromHomeIsADestinationToo() {
        val navigator = Navigator()
        navigator.push(Screen.Lists)
        assertEquals(Back.Rail, navigator.back)

        navigator.push(Screen.ListWorks("l1", "My List"))
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
