package com.mmcdole.kivo.android.settings

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.assertIsFocused
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performKeyInput
import androidx.compose.ui.test.pressKey
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mmcdole.kivo.android.theme.KivoTheme
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

// Settings' two panes: the first section has focus on arrival, moving along
// the sections shows each one's rows without opening anything, Right goes into
// the pane and back to the row it had, and Left comes back to the section.

@OptIn(ExperimentalTestApi::class)
@RunWith(AndroidJUnit4::class)
class SettingsPanesTest {
    @get:Rule
    val compose = createComposeRule()

    /** Two sections, each with two rows tagged by its name. */
    private fun show() {
        compose.setContent {
            KivoTheme {
                SettingsPanes(listOf(SettingsGroup.Account, SettingsGroup.Playback)) { group -> Pane(group) }
            }
        }
    }

    @Composable
    private fun Pane(group: SettingsGroup) {
        val memory = rememberFocusMemory()
        val rows: LazyListScope.() -> Unit = {
            for (row in listOf("first", "second")) {
                item {
                    Column {
                        ActionRow(row, {}, Modifier.focusKey(memory, "${group.name}:$row").testTag("${group.name}:$row"))
                    }
                }
            }
        }
        SettingsPage(group, memory, initial = "${group.name}:first", content = rows)
    }

    private fun focused(tag: String) = compose.waitUntil(5_000) {
        runCatching { compose.onNodeWithTag(tag).assertIsFocused() }.isSuccess
    }

    private fun press(tag: String, key: Key) {
        compose.onNodeWithTag(tag).performKeyInput { pressKey(key) }
        compose.waitForIdle()
    }

    @Test
    fun theFirstSectionHasFocusAndItsRowsShowBesideIt() {
        show()
        focused("settings:Account")
        compose.onNodeWithTag("Account:first").assertExists()
        compose.onNodeWithTag("Playback:first").assertDoesNotExist()
    }

    @Test
    fun movingToASectionShowsItsRowsInPlaceOfTheOthers() {
        show()
        focused("settings:Account")
        press("settings:Account", Key.DirectionDown)
        focused("settings:Playback")
        compose.onNodeWithTag("Playback:first").assertExists()
        compose.onNodeWithTag("Account:first").assertDoesNotExist()
    }

    @Test
    fun rightEntersThePaneAndLeftComesBackToTheSection() {
        show()
        focused("settings:Account")
        press("settings:Account", Key.DirectionRight)
        focused("Account:first")
        press("Account:first", Key.DirectionLeft)
        focused("settings:Account")
    }

    @Test
    fun rightReturnsToTheRowThePaneHad() {
        show()
        focused("settings:Account")
        press("settings:Account", Key.DirectionRight)
        focused("Account:first")
        press("Account:first", Key.DirectionDown)
        focused("Account:second")
        press("Account:second", Key.DirectionLeft)
        focused("settings:Account")
        press("settings:Account", Key.DirectionRight)
        focused("Account:second")
    }

    @Test
    fun downStopsAtTheEndOfThePane() {
        show()
        focused("settings:Account")
        press("settings:Account", Key.DirectionRight)
        focused("Account:first")
        press("Account:first", Key.DirectionDown)
        focused("Account:second")
        press("Account:second", Key.DirectionDown)
        focused("Account:second")
    }
}
