package com.mmcdole.kivo.android.settings

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsFocused
import androidx.compose.ui.test.assertIsNotFocused
import androidx.compose.ui.test.isFocused
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performKeyInput
import androidx.compose.ui.test.pressKey
import androidx.compose.ui.test.requestFocus
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mmcdole.kivo.android.theme.KivoTheme
import com.mmcdole.kivo.android.ui.Choice
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.test.assertEquals

// Settings' list and its panels: a row's line shows under it, a page with
// nothing to press keeps its rows out of focus, switches flip, a setting's
// choices mark the current one, a sign-out asks first, and the credits show.

@RunWith(AndroidJUnit4::class)
class SettingsListTest {
    @get:Rule
    val compose = createComposeRule()

    /** Focuses [node] and presses OK on it, as the remote does. */
    @OptIn(ExperimentalTestApi::class)
    private fun ok(node: SemanticsNodeInteraction) {
        node.requestFocus()
        node.performKeyInput { pressKey(Key.DirectionCenter) }
        compose.waitForIdle()
    }

    @Test
    fun aRowsLineShowsUnderItWhetherOrNotItHasFocus() {
        compose.setContent {
            KivoTheme {
                SettingsList("Audio", rememberFocusMemory(), initial = null) {
                    item { SettingRow("Passthrough", "Auto", {}, supporting = "Sends Dolby and DTS audio to the TV or receiver to decode.") }
                    item { Toggle("Dialogue boost", false) {} }
                }
            }
        }
        compose.onNodeWithText("Sends Dolby and DTS audio to the TV or receiver to decode.").assertExists()
        compose.onNodeWithText("Dialogue boost").requestFocus()
        compose.onNodeWithText("Sends Dolby and DTS audio to the TV or receiver to decode.").assertExists()
    }

    @Test
    fun aPageWithNothingToPressTakesFocus() {
        compose.setContent {
            KivoTheme {
                SettingsList("About", rememberFocusMemory(), initial = null, readOnly = true) {
                    item { InfoRow("App version", "0.1.0") }
                }
            }
        }
        compose.onNodeWithText("App version").requestFocus()
        compose.onNodeWithText("App version").assertIsNotFocused()
        compose.onNode(isFocused()).assertExists()
    }

    @Test
    fun theCreditsShowTmdbsLogoAndNoticeAndTakeFocusAsAPage() {
        compose.setContent {
            KivoTheme {
                CreditsPage()
            }
        }
        compose.onNodeWithText("Credits").assertExists()
        compose.onNodeWithContentDescription("TMDB").assertExists()
        compose.onNodeWithText("This product uses the TMDB API but is not endorsed or certified by TMDB.").assertExists()
        compose.onNodeWithText("Audiobook metadata from Audnexus").assertExists()
        compose.onNodeWithText("Transcoding by jellyfin-ffmpeg (GPL)").assertExists()
        compose.waitUntil(5_000) { runCatching { compose.onNode(isFocused()).assertExists() }.isSuccess }
    }

    @Test
    fun okFlipsASwitch() {
        var on by mutableStateOf(false)
        compose.setContent {
            KivoTheme {
                SettingsList("Playback", rememberFocusMemory(), initial = null) {
                    item { Toggle("Remember choices per show", on) { on = it } }
                }
            }
        }
        ok(compose.onNodeWithTag("toggle:Remember choices per show"))
        assertEquals(true, on)
    }

    @Test
    fun aSettingsChoicesOpenOnTheCurrentOneAndPickingOneCloses() {
        var picked: String? = null
        var open by mutableStateOf(true)
        val choices = listOf(Choice("Auto", "Auto"), Choice("On", "On"), Choice("Off", "Off"))
        compose.setContent {
            KivoTheme {
                if (open) PickingPanel(Picking("Passthrough", choices, "On") { picked = it }) { open = false }
            }
        }
        compose.waitUntil(5_000) { runCatching { compose.onNodeWithText("On").assertIsFocused() }.isSuccess }
        compose.onNodeWithText("On").assert(SemanticsMatcher.expectValue(SemanticsProperties.Selected, true))
        ok(compose.onNodeWithText("Off"))
        assertEquals("Off", picked)
        assertEquals(false, open)
    }

    @Test
    fun aSignOutAsksFirstWithCancelFocused() {
        var confirmed = false
        var open by mutableStateOf(true)
        compose.setContent {
            KivoTheme {
                if (open) Confirm("Sign out Living Room?", "Sign out", null, onConfirm = { confirmed = true }, onDismiss = { open = false })
            }
        }
        compose.waitUntil(5_000) { runCatching { compose.onNodeWithText("Cancel").assertIsFocused() }.isSuccess }
        ok(compose.onNodeWithText("Cancel"))
        assertEquals(false, confirmed)
        assertEquals(false, open)
    }
}
