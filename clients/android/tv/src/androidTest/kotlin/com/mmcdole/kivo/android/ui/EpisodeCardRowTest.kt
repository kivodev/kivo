package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.focusable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.assertIsFocused
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performKeyInput
import androidx.compose.ui.test.pressKey
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mmcdole.kivo.android.theme.KivoTheme
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkSummary
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.test.assertEquals

// A season's row: one focus target a card with two stops, OK on the thumbnail
// playing and OK on the text opening its page, each stop kept along the row
// and left behind as focus leaves it.

@OptIn(ExperimentalTestApi::class)
@RunWith(AndroidJUnit4::class)
class EpisodeCardRowTest {
    @get:Rule
    val compose = createComposeRule()

    private val navigator = Navigator()

    @Test
    fun okOnTheThumbnailPlays() {
        show(entry = "e2")
        press(Key.Enter)
        assertEquals(Screen.Player("e2"), navigator.current.screen)
    }

    @Test
    fun okOnTheTextOpensThePage() {
        show(entry = "e2")
        press(Key.DirectionDown)
        card("e2").assertIsFocused()
        press(Key.Enter)
        assertEquals(Screen.Work("e2"), navigator.current.screen)
    }

    @Test
    fun theStopHoldsAlongTheRow() {
        show(entry = "e1")
        press(Key.DirectionDown)
        press(Key.DirectionRight)
        card("e2").assertIsFocused()
        press(Key.Enter)
        assertEquals(Screen.Work("e2"), navigator.current.screen)
    }

    @Test
    fun upFromTheTextReturnsToTheThumbnail() {
        show(entry = "e2")
        press(Key.DirectionDown)
        press(Key.DirectionUp)
        card("e2").assertIsFocused()
        press(Key.Enter)
        assertEquals(Screen.Player("e2"), navigator.current.screen)
    }

    @Test
    fun leavingTheRowResetsTheStopAndComingBackEntersOnTheCardLeft() {
        show(entry = "e2")
        press(Key.DirectionDown)
        press(Key.DirectionDown)
        compose.onNodeWithTag("below").assertIsFocused()
        press(Key.DirectionUp)
        card("e2").assertIsFocused()
        press(Key.Enter)
        assertEquals(Screen.Player("e2"), navigator.current.screen)
    }

    @Test
    fun anUnavailableEpisodesThumbnailOpensItsPage() {
        show(entry = "e3")
        press(Key.Enter)
        assertEquals(Screen.Work("e3"), navigator.current.screen)
    }

    @Test
    fun thePlayKeyPlaysFromTheText() {
        show(entry = "e2")
        press(Key.DirectionDown)
        press(Key.MediaPlay)
        assertEquals(Screen.Player("e2"), navigator.current.screen)
    }

    private fun show(entry: String) {
        val group = FocusRequester()
        compose.setContent {
            KivoTheme {
                Column {
                    val memory = remember { FocusMemory(null) }
                    EpisodeCardRow(episodes, entry, null, memory, group, navigator)
                    androidx.compose.foundation.layout.Box(Modifier.testTag("below").size(40.dp).focusable())
                }
            }
        }
        compose.runOnIdle { group.requestFocus() }
        card(entry).assertIsFocused()
    }

    private fun card(id: String) = compose.onNodeWithTag("episode:$id")

    private fun press(key: Key) {
        compose.onRoot().performKeyInput { pressKey(key) }
        compose.waitForIdle()
    }

    private companion object {
        fun episode(id: String, number: Int, available: Boolean = true) = WorkSummary(
            id = id, kind = "episode", title = id, matchState = "matched", available = available, images = WorkImages(),
            durationMs = 2_640_000, episode = EpisodeInfo(1, number, "series", "Series", seasonId = "season", overview = "What happens."),
        )

        val episodes = listOf(episode("e1", 1), episode("e2", 2), episode("e3", 3, available = false))
    }
}
