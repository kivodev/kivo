package com.mmcdole.kivo.android.mobile.linkdevice

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mmcdole.kivo.android.mobile.theme.KivoMobileTheme
import com.mmcdole.kivo.api.models.ClientPlatform
import com.mmcdole.kivo.api.models.PendingDevice
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.test.assertEquals
import kotlin.time.Clock

// Link a device's steps, drawn alone: a typed code, the device asking, and the answer.

@RunWith(AndroidJUnit4::class)
class LinkDeviceFlowTest {
    @get:Rule
    val rule = createComposeRule()

    @Test
    fun aTypedCodeShowsTheDeviceAskingAndAllowSignsItIn() {
        val looked = mutableListOf<String>()
        val answers = mutableListOf<Boolean>()
        val tv = PendingDevice("Living Room", nearby = false, startedAt = Clock.System.now(), platform = ClientPlatform.AndroidTv)
        var state by mutableStateOf(LinkUiState(username = "anna", serverName = "Den"))
        rule.setContent {
            KivoMobileTheme {
                LinkDeviceContent(
                    state,
                    camera = false,
                    onShow = {},
                    onLookUp = {
                        looked += it
                        state = state.copy(step = LinkStep.Confirm("K7Q2M9", tv))
                    },
                    onAnswer = {
                        answers += it
                        state = state.copy(step = LinkStep.Done("Living Room", it))
                    },
                    onDone = {},
                )
            }
        }
        rule.onNodeWithTag("scan").assertDoesNotExist()
        rule.onNodeWithTag("continue").assertIsNotEnabled()
        rule.onNodeWithTag("codeField").performTextInput("k7q2m9")
        rule.onNodeWithTag("continue").assertIsEnabled().performClick()
        rule.onNodeWithTag("linkQuestion").assertTextEquals("Allow Living Room to sign in as anna?")
        rule.onNodeWithTag("linkContext").assertTextEquals("Android TV · Not on your home network · Just now")
        rule.onNodeWithTag("linkWarning").assertIsDisplayed()
        rule.onNodeWithTag("allow").performClick()
        rule.onNodeWithTag("linkDone").assertTextEquals("Living Room is signed in")
        rule.runOnIdle {
            assertEquals(listOf("K7Q2M9"), looked)
            assertEquals(listOf(true), answers)
        }
    }

    @Test
    fun aCodeThatDidntWorkSaysSo() {
        rule.setContent {
            KivoMobileTheme {
                LinkDeviceContent(LinkUiState(message = LinkMessage.CodeFailed), camera = true, onShow = {}, onLookUp = {}, onAnswer = {}, onDone = {})
            }
        }
        rule.onNodeWithTag("scan").assertIsDisplayed()
        rule.onNodeWithTag("linkMessage").assertTextEquals("That code didn't work. Check the other device for a new one.")
    }
}
