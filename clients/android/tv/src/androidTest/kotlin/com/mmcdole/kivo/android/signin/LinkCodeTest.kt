package com.mmcdole.kivo.android.signin

import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.mmcdole.kivo.android.TestDevice
import com.mmcdole.kivo.android.theme.KivoTheme
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.session.LinkProgress
import com.mmcdole.kivo.core.session.KivoClient
import com.mmcdole.kivo.core.session.KivoSession
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File
import java.util.UUID
import kotlin.test.assertEquals

// The TV's sign-in code: shown grouped for reading across the room, replaced
// with a new one when the other device declines, and signed in once allowed.

@RunWith(AndroidJUnit4::class)
class LinkCodeTest {
    @get:Rule
    val compose = createComposeRule()

    private val context = InstrumentationRegistry.getInstrumentation().targetContext
    private val preferences = File(context.cacheDir, "link-code-test-${UUID.randomUUID()}.preferences_pb")

    @After
    fun deletePreferences() {
        preferences.delete()
    }

    private fun waiting(code: String) = LinkProgress.Waiting(code, "http://kivo.local:8484/link?code=$code&server=s-1")

    /** A session on a server that doesn't answer, as a signed-in code opens. */
    private fun session(): KivoSession {
        val core = KivoClient(TestDevice(context, preferences), MockEngine { respond("", HttpStatusCode.NotFound) })
        return runBlocking { core.restore(SavedServer("s-1", "Basement", listOf("http://kivo.local:8484"))) }
    }

    /** A device code per call, each the next of [links]. */
    private fun links(vararg links: Flow<LinkProgress>): () -> Flow<LinkProgress> {
        var next = 0
        return { links[next++] }
    }

    private fun show(link: () -> Flow<LinkProgress>, onSignedIn: suspend (KivoSession) -> Unit = {}) {
        compose.setContent {
            KivoTheme { LinkCode("Basement", link, onSignedIn, onPassword = {}, onChangeServer = {}) }
        }
    }

    private fun waitForCode(code: String) = compose.waitUntil(5_000) {
        runCatching { compose.onNodeWithTag("linkCode").assertTextEquals(code) }.isSuccess
    }

    @Test
    fun aDeclinedCodeSaysSoOverANewOne() {
        show(
            links(
                flow {
                    emit(waiting("ABC234"))
                    emit(LinkProgress.Denied)
                },
                flow {
                    emit(waiting("XYZ789"))
                    awaitCancellation()
                },
            ),
        )

        waitForCode("XYZ-789")
        compose.onNodeWithTag("signInDeclined").assertTextEquals("Sign-in was declined.")
    }

    @Test
    fun theCodeIsShownUntilItsApproved() {
        var signedIn = 0
        val session = session()
        show(
            links(
                flow {
                    emit(waiting("ABC234"))
                    emit(LinkProgress.SignedIn(session))
                },
            ),
            onSignedIn = { if (it === session) signedIn++ },
        )

        waitForCode("ABC-234")
        compose.waitUntil(5_000) { signedIn == 1 }
        compose.onNodeWithTag("signInDeclined").assertDoesNotExist()
        assertEquals(1, signedIn)
    }
}
