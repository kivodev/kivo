package com.mmcdole.kivo.android.mobile.signin

import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.SERVER
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.core.discovery.FoundServer
import com.mmcdole.kivo.core.session.KivoSession
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class SignInViewModelTest : CoreTest() {
    private val opened = CompletableDeferred<KivoSession>()

    private fun model(found: List<FoundServer> = emptyList()) =
        SignInViewModel(client, { if (found.isEmpty()) emptyFlow() else flowOf(*found.toTypedArray()) }, "Phone", null) { opened.complete(it) }

    @Test
    fun aTypedAddressGoesToItsPasswordAndSignsIn() {
        val model = model()
        model.setAddress(SERVER)
        model.connect()
        val state = awaitValue(model.state) { it.step is SignInStep.Password }
        assertEquals("Den", (state.step as SignInStep.Password).server.info.name)
        model.signIn("anna ", "pw")
        val session = runBlocking { withTimeout(5_000) { opened.await() } }
        assertTrue(session.isSignedIn)
    }

    @Test
    fun noServerAtTheAddressSaysSo() {
        // Something answers, but not a Kivo server.
        server.on("GET", "/api/v1/server") { respond("not here", HttpStatusCode.NotFound) }
        val model = model()
        model.setAddress("https://nothing.test")
        model.connect()
        assertEquals(SignInError.NoServer, awaitValue(model.state) { it.error != null }.error)
    }

    @Test
    fun aLoneServerFoundGoesStraightToItsPassword() {
        val den = FoundServer(SERVER, ServerInfo("s1", "Den", "1.0"))
        val model = model(listOf(den))
        model.search()
        val state = awaitValue(model.state) { it.step is SignInStep.Password }
        assertEquals(den, (state.step as SignInStep.Password).server)
        assertTrue(model.back())
        assertEquals(SignInStep.Find, awaitValue(model.state) { it.step == SignInStep.Find }.step)
    }

    @Test
    fun codesAreGroupedForReading() {
        assertEquals("K7Q-2M9", groupedCode("K7Q2M9"))
    }
}
