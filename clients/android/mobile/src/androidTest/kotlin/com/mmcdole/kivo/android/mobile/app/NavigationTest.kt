package com.mmcdole.kivo.android.mobile.app

import androidx.compose.material3.Text
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.navigation
import androidx.navigation.compose.rememberNavController
import androidx.navigation.NavHostController
import androidx.navigation.toRoute
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.test.assertEquals

// Each tab keeps its own back stack: the app's routes and MobileNav over
// stand-in screens that name where they are.

@RunWith(AndroidJUnit4::class)
class NavigationTest {
    @get:Rule
    val rule = createComposeRule()

    private lateinit var controller: NavHostController
    private lateinit var nav: MobileNav

    private fun current() = nav.tab

    private fun show() {
        rule.setContent {
            controller = rememberNavController()
            nav = remember(controller) { MobileNav(controller) }
            val entry by controller.currentBackStackEntryAsState()
            LaunchedEffect(entry) { nav.follow(entry) }
            NavHost(controller, startDestination = HomeTab) {
                navigation<HomeTab>(startDestination = HomeRoute) { composable<HomeRoute> { Text("home") } }
                navigation<LibrariesTab>(startDestination = LibrariesRoute) { composable<LibrariesRoute> { Text("libraries") } }
                navigation<ListsTab>(startDestination = ListsRoute) { composable<ListsRoute> { Text("lists") } }
                navigation<DownloadsTab>(startDestination = DownloadsRoute) { composable<DownloadsRoute> { Text("downloads") } }
                navigation<SearchTab>(startDestination = SearchRoute) { composable<SearchRoute> { Text("search") } }
                composable<WorkRoute> { Text("work ${it.toRoute<WorkRoute>().id}") }
                composable<LibraryRoute> { Text("library ${it.toRoute<LibraryRoute>().id}") }
            }
        }
    }

    private fun onMain(block: () -> Unit) {
        rule.runOnIdle(block)
        rule.waitForIdle()
    }

    @Test
    fun eachTabComesBackAsItWasLeft() {
        show()
        onMain { nav.open(WorkRoute("a")) }
        rule.onNodeWithText("work a").assertExists()
        onMain { nav.select(Tab.Libraries) }
        onMain { nav.open(LibraryRoute("movies")) }
        rule.onNodeWithText("library movies").assertExists()
        assertEquals(Tab.Libraries, rule.runOnIdle { current() })

        onMain { nav.select(Tab.Home) }
        rule.onNodeWithText("work a").assertExists()
        assertEquals(Tab.Home, rule.runOnIdle { current() })

        onMain { nav.select(Tab.Libraries) }
        rule.onNodeWithText("library movies").assertExists()
    }

    @Test
    fun theTabShowingGoesBackToItsFirstScreen() {
        show()
        onMain { nav.open(WorkRoute("a")) }
        onMain { nav.open(WorkRoute("b")) }
        onMain { nav.select(Tab.Home) }
        rule.onNodeWithText("home").assertExists()
    }

    @Test
    fun backFromAnotherTabsFirstScreenIsHome() {
        show()
        onMain { nav.select(Tab.Search) }
        rule.onNodeWithText("search").assertExists()
        onMain { nav.back() }
        rule.onNodeWithText("home").assertExists()
        assertEquals(Tab.Home, rule.runOnIdle { current() })
    }

    @Test
    fun aPinnedLibraryOpensOnTheLibrariesTab() {
        show()
        onMain { nav.openLibrary("books") }
        rule.onNodeWithText("library books").assertExists()
        assertEquals(Tab.Libraries, rule.runOnIdle { current() })
        onMain { nav.back() }
        rule.onNodeWithText("libraries").assertExists()
    }
}
