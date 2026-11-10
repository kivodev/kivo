package com.mmcdole.kivo.android.mobile.signin

import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.SERVER
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.core.Compatibility
import com.mmcdole.kivo.core.MIN_SERVER_VERSION
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

    override val appVersion get() = "0.5.0"

    /** The server answers as [version], supporting apps from [minAppVersion]. */
    private fun serverIs(version: String, minAppVersion: String?) =
        server.get("/api/v1/server", ServerInfo.serializer()) { ServerInfo("s1", "Den", version, minAppVersion = minAppVersion) }

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
    fun aServerTooOldForTheAppSaysSoInPlaceOfItsPassword() {
        serverIs("0.2.0", "0.1.0")
        val model = model()
        model.setAddress(SERVER)
        model.connect()
        val step = awaitValue(model.state) { it.step is SignInStep.Incompatible }.step as SignInStep.Incompatible
        assertEquals(Compatibility.ServerTooOld(serverVersion = "0.2.0", minServerVersion = MIN_SERVER_VERSION, appVersion = "0.5.0"), step.problem)
        assertEquals("Den", step.server.info.name)

        // Change server goes back to choosing one.
        assertTrue(model.back())
        assertEquals(SignInStep.Find, awaitValue(model.state) { it.step == SignInStep.Find }.step)
    }

    @Test
    fun anAppTooOldForTheServerSaysSoAndGoesOnOnceTryAgainFindsItSupported() {
        serverIs("0.9.0", "0.6.0")
        val model = model()
        model.setAddress(SERVER)
        model.connect()
        val step = awaitValue(model.state) { it.step is SignInStep.Incompatible }.step as SignInStep.Incompatible
        assertEquals(Compatibility.AppTooOld(appVersion = "0.5.0", minAppVersion = "0.6.0"), step.problem)

        // Still unsupported, Try again stays.
        model.retryCompatibility()
        assertTrue(awaitValue(model.state) { !it.busy }.step is SignInStep.Incompatible)

        serverIs("0.9.0", "0.5.0")
        model.retryCompatibility()
        assertEquals("Den", (awaitValue(model.state) { it.step is SignInStep.Password }.step as SignInStep.Password).server.info.name)
    }

    @Test
    fun aServerFoundOnTheNetworkThatTheAppDoesntWorkWithSaysSo() {
        val old = FoundServer(SERVER, ServerInfo("s1", "Den", "0.2.0"))
        val model = model(listOf(old))
        model.search()
        assertTrue(awaitValue(model.state) { it.step is SignInStep.Incompatible }.step is SignInStep.Incompatible)
    }

    @Test
    fun aServerThatSaysNothingOfVersionsGoesToItsPassword() {
        serverIs("dev", null)
        val model = model()
        model.setAddress(SERVER)
        model.connect()
        assertTrue(awaitValue(model.state) { it.step is SignInStep.Password }.step is SignInStep.Password)
    }

    @Test
    fun codesAreGroupedForReading() {
        assertEquals("K7Q-2M9", groupedCode("K7Q2M9"))
    }
}
