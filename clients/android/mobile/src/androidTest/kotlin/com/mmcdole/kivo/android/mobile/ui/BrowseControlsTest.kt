package com.mmcdole.kivo.android.mobile.ui

import androidx.compose.foundation.layout.height
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.click
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.longClick
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeDown
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mmcdole.kivo.android.mobile.library.LetterRail
import com.mmcdole.kivo.android.mobile.theme.KivoMobileTheme
import com.mmcdole.kivo.api.models.UserState
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkSummary
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.test.assertEquals
import kotlin.test.assertTrue

// The letter rail and a card, with what TalkBack has of them, and a card's long-press sheet, drawn alone.

@RunWith(AndroidJUnit4::class)
class BrowseControlsTest {
    @get:Rule
    val rule = createComposeRule()

    private val letters = listOf("A", "B", "C", "D", "E")

    @Test
    fun aTapOnTheRailJumpsToTheLetterUnderIt() {
        val jumps = mutableListOf<String>()
        rule.setContent { KivoMobileTheme { LetterRail(letters, "A", { jumps += it }, Modifier.height(250.dp)) } }
        rule.onNodeWithTag("letters").performTouchInput { click(androidx.compose.ui.geometry.Offset(centerX, height * 0.9f)) }
        rule.runOnIdle { assertEquals("E", jumps.last()) }
    }

    @Test
    fun aDragAlongTheRailPassesEachLetterOnce() {
        val jumps = mutableListOf<String>()
        rule.setContent { KivoMobileTheme { LetterRail(letters, "A", { jumps += it }, Modifier.height(250.dp)) } }
        rule.onNodeWithTag("letters").performTouchInput { swipeDown(startY = 2f, endY = height - 2f, durationMillis = 600) }
        rule.runOnIdle { assertEquals(letters, jumps) }
    }

    @Test
    fun talkBackHasTheRailAsOneAdjustableControl() {
        val jumps = mutableListOf<String>()
        rule.setContent { KivoMobileTheme { LetterRail(letters, "B", { jumps += it }, Modifier.height(250.dp)) } }
        rule.onNodeWithTag("letters")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.ContentDescription, listOf("Jump to letter")))
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, "B"))
            .performSemanticsAction(SemanticsActions.SetProgress) { it(3f) }
        rule.runOnIdle { assertEquals(listOf("D"), jumps) }
    }

    @Test
    fun talkBackReadsACardsTitleLineAndProgressAsOne() {
        val work = WorkSummary(
            id = "m1", kind = "movie", title = "Charade", matchState = "matched", available = true, images = WorkImages(), year = 1963,
            durationMs = 100_000, userState = UserState(played = false, playCount = 0, positionMs = 30_000),
        )
        rule.setContent { KivoMobileTheme { WorkCard(work, CardShape.Poster, 116.dp, onClick = {}, onLongClick = null) } }
        rule.onNodeWithTag("card:Charade").assert(SemanticsMatcher.expectValue(SemanticsProperties.ContentDescription, listOf("Charade, 1963, 30% watched")))
        rule.onAllNodesWithText("Charade").assertCountEquals(0)
    }

    @Test
    fun aLongPressOpensTheCardsActionsAndOneRunsOnTheWork() {
        val work = WorkSummary(
            id = "m1", kind = "movie", title = "Charade", matchState = "matched", available = true, images = WorkImages(),
            durationMs = 100_000, userState = UserState(played = false, playCount = 0, positionMs = 30_000),
        )
        val done = mutableListOf<Pair<WorkAction, String>>()
        rule.setContent {
            KivoMobileTheme {
                WorkMenuHost({ action, w -> done += action to w.id }) { open ->
                    WorkCard(work, CardShape.Landscape, 260.dp, onClick = {}, onLongClick = { open(WorkMenu(work, CONTINUE_WATCHING)) })
                }
            }
        }
        rule.onNodeWithTag("card:Charade").performTouchInput { longClick() }
        rule.onNodeWithTag("workActions").assertExists()
        rule.onAllNodesWithTag("action:RemoveFromContinue").assertCountEquals(1)
        rule.onAllNodesWithTag("action:PlayFromStart").assertCountEquals(1)
        rule.onNodeWithTag("action:AddToList").performClick()
        rule.waitUntil(5_000) { done.isNotEmpty() }
        rule.onNodeWithTag("workActions").assertDoesNotExist()
        assertTrue(done.single() == WorkAction.AddToList to "m1")
    }
}
