package com.mmcdole.kivo.android.mobile.player

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mmcdole.kivo.android.mobile.theme.KivoMobileTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.test.assertEquals
import kotlin.test.assertNull

// The player's popups, drawn alone: a row's list replaces the popup's rows
// in place, under a row of its name, and Back goes from the list to the
// rows and then closes the popup.

@RunWith(AndroidJUnit4::class)
class PlayerPopupTest {
    @get:Rule
    val rule = createAndroidComposeRule<ComponentActivity>()

    private val menus = MenuState()
    private val chosen = mutableListOf<String>()

    private fun show() {
        rule.setContent {
            KivoMobileTheme {
                Box(Modifier.fillMaxSize()) {
                    val anchor = remember { Rect(600f, 900f, 700f, 1000f) }
                    PlayerPopup(
                        menus,
                        anchor,
                        260.dp,
                        pageOf = { _, sub ->
                            when (sub) {
                                SubMenu.Speed -> MenuPage(listOf(MenuItem("1.0", "Normal", ticked = true), MenuItem("1.5", "1.5×")), back = "Speed")
                                else -> MenuPage(
                                    listOf(
                                        MenuItem(SubMenu.Quality.name, "Quality", "Auto", opens = true),
                                        MenuItem(SubMenu.Speed.name, "Speed", "Normal", opens = true),
                                        MenuItem("dialogueBoost", "Dialogue boost", switch = false),
                                    ),
                                )
                            }
                        },
                        onChoose = { _, _, id ->
                            chosen += id
                            if (id != "dialogueBoost") menus.close()
                        },
                    )
                }
            }
        }
        rule.runOnUiThread { menus.open(PlayerMenu.Settings) }
    }

    private fun back() = rule.runOnUiThread { rule.activity.onBackPressedDispatcher.onBackPressed() }

    @Test
    fun aRowsListReplacesThePopupsRowsUnderItsName() {
        show()
        rule.onNodeWithText("Quality").assertIsDisplayed()
        rule.onNodeWithTag("menu:Speed").performClick()
        rule.onNodeWithTag("menuBack").assertIsDisplayed()
        rule.onNodeWithText("1.5×").assertIsDisplayed()
        rule.onNodeWithText("Quality").assertDoesNotExist()
        rule.onNodeWithTag("menuBack").performClick()
        rule.onNodeWithText("Quality").assertIsDisplayed()
        rule.runOnIdle { assertNull(menus.sub) }
    }

    @Test
    fun backGoesFromAListToThePopupsRowsThenCloses() {
        show()
        rule.onNodeWithTag("menu:Speed").performClick()
        back()
        rule.onNodeWithText("Dialogue boost").assertIsDisplayed()
        back()
        rule.onNodeWithTag("playerMenu").assertDoesNotExist()
        rule.runOnIdle { assertNull(menus.open) }
    }

    @Test
    fun aChoiceInAListClosesThePopupAndASwitchKeepsItOpen() {
        show()
        rule.onNodeWithTag("menu:dialogueBoost").performClick()
        rule.onNodeWithTag("playerMenu").assertIsDisplayed()
        rule.onNodeWithTag("menu:Speed").performClick()
        rule.onNodeWithText("1.5×").performClick()
        rule.onNodeWithTag("playerMenu").assertDoesNotExist()
        rule.runOnIdle { assertEquals(listOf("dialogueBoost", "1.5"), chosen) }
    }

    @Test
    fun aTapOutsideClosesThePopup() {
        show()
        rule.onNodeWithTag("menuScrim").performClick()
        rule.onNodeWithTag("playerMenu").assertDoesNotExist()
    }
}
