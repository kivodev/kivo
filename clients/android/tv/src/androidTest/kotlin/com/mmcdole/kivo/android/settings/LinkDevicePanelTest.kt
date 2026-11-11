package com.mmcdole.kivo.android.settings

import androidx.compose.ui.input.key.Key
import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.test.assertIsFocused
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performKeyInput
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.pressKey
import androidx.compose.ui.test.requestFocus
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mmcdole.kivo.android.theme.KivoTheme
import com.mmcdole.kivo.api.models.ClientPlatform
import com.mmcdole.kivo.api.models.PendingDevice
import com.mmcdole.kivo.core.Done
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.test.assertEquals
import kotlin.time.Clock

// Link a device on the TV: the code typed with the remote's keyboard, the
// question with what the server knows of the asking device, Don't allow
// focused first, and the answer sent as this user.

@RunWith(AndroidJUnit4::class)
class LinkDevicePanelTest {
    @get:Rule
    val compose = createComposeRule()

    /** Answers like the server, keeping what was asked. */
    private class FakeLink(private val asking: PendingDevice) : LinkActions {
        val looked = mutableListOf<String>()
        val approved = mutableListOf<String>()
        val denied = mutableListOf<String>()

        override suspend fun pendingDevice(code: String) = asking.also { looked += code }

        override suspend fun approve(code: String): Done<Unit> {
            approved += code
            return Done.Ok(Unit)
        }

        override suspend fun deny(code: String): Done<Unit> {
            denied += code
            return Done.Ok(Unit)
        }
    }

    @OptIn(ExperimentalTestApi::class)
    private fun ok(node: SemanticsNodeInteraction) {
        node.requestFocus()
        node.performKeyInput { pressKey(Key.DirectionCenter) }
        compose.waitForIdle()
    }

    private fun waitForFocus(tag: String) =
        compose.waitUntil(5_000) { runCatching { compose.onNodeWithTag(tag).assertIsFocused() }.isSuccess }

    private fun open(link: FakeLink) {
        compose.setContent {
            KivoTheme { LinkDevicePanel("anna", link, onDismiss = {}) }
        }
        compose.onNodeWithTag("field:Code").performTextInput("k7q-2m9")
        ok(compose.onNodeWithTag("linkContinue"))
        waitForFocus("dontAllow")
    }

    @Test
    fun aTypedCodeAsksWithWhatTheServerKnowsAndAllowSignsItIn() {
        val link = FakeLink(PendingDevice("Living Room", platform = ClientPlatform.AndroidTv, nearby = false, startedAt = Clock.System.now()))
        open(link)

        assertEquals(listOf("K7Q2M9"), link.looked)
        compose.onNodeWithTag("linkContext").assertTextEquals("Android TV · Not on your home network · Just now")
        compose.onNodeWithTag("linkWarning").assertExists()
        ok(compose.onNodeWithTag("allow"))

        assertEquals(listOf("K7Q2M9"), link.approved)
        compose.onNodeWithTag("linkAnswered").assertTextEquals("Living Room is signed in")
    }

    @Test
    fun dontAllowDeniesIt() {
        val link = FakeLink(PendingDevice("Den", platform = ClientPlatform.IPad, nearby = true, startedAt = Clock.System.now()))
        open(link)

        compose.onNodeWithTag("linkContext").assertTextEquals("iPad · On your home network · Just now")
        compose.onNodeWithTag("linkWarning").assertDoesNotExist()
        ok(compose.onNodeWithTag("dontAllow"))

        assertEquals(listOf("K7Q2M9"), link.denied)
        assertEquals(emptyList(), link.approved)
        compose.onNodeWithTag("linkAnswered").assertTextEquals("Den wasn't signed in")
    }
}
