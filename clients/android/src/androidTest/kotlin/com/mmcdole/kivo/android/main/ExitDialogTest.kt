package com.mmcdole.kivo.android.main

import androidx.compose.ui.test.assertIsFocused
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mmcdole.kivo.android.theme.KivoTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.test.assertEquals

// Asking before leaving: No, the safe choice, has focus first, and only Exit leaves.

@RunWith(AndroidJUnit4::class)
class ExitDialogTest {
    @get:Rule
    val compose = createComposeRule()

    private var exited = 0
    private var dismissed = 0

    private fun show() {
        compose.setContent {
            KivoTheme { ExitDialog(onExit = { exited++ }, onDismiss = { dismissed++ }) }
        }
    }

    @Test
    fun noHasFocusFirst() {
        show()
        compose.onNodeWithText("Exit Kivo?").assertExists()
        compose.waitUntil(5_000) { runCatching { compose.onNodeWithTag("exit:no").assertIsFocused() }.isSuccess }
    }

    @Test
    fun noStaysAndExitLeaves() {
        show()
        compose.onNodeWithTag("exit:no").performClick()
        assertEquals(1, dismissed)
        assertEquals(0, exited)
        compose.onNodeWithTag("exit:yes").performClick()
        assertEquals(1, exited)
    }
}
