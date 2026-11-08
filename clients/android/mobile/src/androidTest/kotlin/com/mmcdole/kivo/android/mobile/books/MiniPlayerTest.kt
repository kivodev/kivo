package com.mmcdole.kivo.android.mobile.books

import androidx.compose.ui.test.assertHeightIsEqualTo
import androidx.compose.ui.test.getBoundsInRoot
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.width
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mmcdole.kivo.android.mobile.theme.KivoMobileTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.test.assertEquals
import kotlin.test.assertTrue

// The mini player, drawn alone: its lines, its buttons, and the book's progress along its foot.

@RunWith(AndroidJUnit4::class)
class MiniPlayerTest {
    @get:Rule
    val rule = createComposeRule()

    private val book = MiniPlayerUiState(shown = true, title = "The Martian", chapterIndex = 11, remainingMs = 2 * 3_600_000L, fraction = 0.25f, playing = true)

    @Test
    fun itShowsTheBookAndItsButtonsDoWhatTheySay() {
        val done = mutableListOf<String>()
        rule.setContent {
            KivoMobileTheme {
                MiniPlayer(book, onOpen = { done += "open" }, onBack = { done += "back" }, onToggle = { done += "toggle" })
            }
        }
        rule.onNodeWithTag("miniPlayer").assertHeightIsEqualTo(64.dp)
        rule.onNodeWithText("The Martian").assertExists()
        rule.onNodeWithText("Chapter 12", substring = true).assertExists()
        rule.onNodeWithContentDescription("Pause").assertExists()
        rule.onNodeWithTag("miniBack").performClick()
        rule.onNodeWithTag("miniPlay").performClick()
        rule.onNodeWithText("The Martian").performClick()
        rule.runOnIdle { assertEquals(listOf("back", "toggle", "open"), done) }
    }

    @Test
    fun theProgressLineRunsAsFarAsTheBookHas() {
        rule.setContent { KivoMobileTheme { MiniPlayer(book, onOpen = {}, onBack = {}, onToggle = {}) } }
        val bar = rule.onNodeWithTag("miniPlayer").getBoundsInRoot()
        val line = rule.onNodeWithTag("miniProgress", useUnmergedTree = true)
        line.assertHeightIsEqualTo(2.dp)
        val width = line.getBoundsInRoot().width
        assertTrue(width > bar.width * 0.24f && width < bar.width * 0.26f, "progress $width of ${bar.width}")
    }

    @Test
    fun pausedItOffersPlay() {
        rule.setContent { KivoMobileTheme { MiniPlayer(book.copy(playing = false), onOpen = {}, onBack = {}, onToggle = {}) } }
        rule.onNodeWithContentDescription("Play").assertExists()
    }
}
