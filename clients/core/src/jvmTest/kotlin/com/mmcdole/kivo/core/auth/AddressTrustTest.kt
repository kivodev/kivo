package com.mmcdole.kivo.core.auth

import androidx.datastore.preferences.core.stringPreferencesKey
import com.mmcdole.kivo.api.models.CurrentUser
import com.mmcdole.kivo.api.models.PlaybackPreferences
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.api.models.ServerProof
import com.mmcdole.kivo.api.models.SubtitleMode
import com.mmcdole.kivo.api.models.UserLimits
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.MemoryPreferences
import com.mmcdole.kivo.core.MemoryStore
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.problem
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockEngineConfig
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import java.io.IOException
import java.security.KeyPair
import java.security.KeyPairGenerator
import java.security.Signature
import java.util.Base64
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.coroutines.EmptyCoroutineContext
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

// The token goes only to an address that has proven, on the device's
// network as it is now, that it's the server: an HTTPS one, or a plain-HTTP
// one whose answer to a probe carries the server's signature by its key.

private const val LOCAL = "http://192.168.1.20:8484"
private const val PUBLIC = "https://kivo.example.com"

/** A session token, its kind and then 32 bytes base64url, as the server issues them. */
private val TOKEN = "kivo_s_" + Base64.getUrlEncoder().withoutPadding().encodeToString(ByteArray(32) { it.toByte() })

private val anna = CurrentUser("u1", "anna", admin = false, limits = UserLimits(), playback = PlaybackPreferences(SubtitleMode.Smart, true, true, 1.0))

/** How the plain-HTTP address answers a probe: as the server, or as one of the impostors. */
private enum class Answer { Server, OtherKey, NoProof, NotLocal, OtherHost }

private fun b64(b: ByteArray) = Base64.getUrlEncoder().withoutPadding().encodeToString(b)

private fun ed25519(): KeyPair = KeyPairGenerator.getInstance("Ed25519").generateKeyPair()

/** A key pair's public key as the server gives it: its 32 raw bytes, the end of its X.509 encoding. */
private fun KeyPair.encoded(): String = b64(public.encoded.takeLast(32).toByteArray())

private val SERVER = ed25519()
private val OTHER = ed25519()

/** The server's proof, worked out apart from the app's, with the JDK's Ed25519. */
private fun proof(key: KeyPair, challenge: ByteArray, host: String, local: Boolean): ServerProof {
    val out = java.io.ByteArrayOutputStream()
    out.write("kivo-probe-v2".toByteArray())
    for (f in listOf(challenge, "server-1".toByteArray(), host.toByteArray())) {
        out.write(f.size shr 8)
        out.write(f.size and 0xff)
        out.write(f)
    }
    out.write(if (local) 1 else 0)
    val signature = Signature.getInstance("Ed25519").apply { initSign(key.private) }.apply { update(out.toByteArray()) }.sign()
    return ServerProof(b64(signature), key.encoded(), host, local)
}

/** The server at its addresses, the local one answering probes as [answer] says, and every request as "host path auth". */
private class Net(scope: TestScope) {
    val up = mutableSetOf(LOCAL, PUBLIC)
    var answer = Answer.Server
    val seen: MutableList<String> = CopyOnWriteArrayList()

    val engine = MockEngine(
        MockEngineConfig().apply {
            dispatcher = StandardTestDispatcher(scope.testScheduler)
            addHandler { request ->
                val auth = request.headers[HttpHeaders.Authorization]
                seen += "${request.url.host} ${request.url.encodedPath} ${auth ?: "-"}"
                if (up.none { it.contains(request.url.host) }) throw IOException("no route to host")
                when (request.url.encodedPath) {
                    "/api/v1/server" -> {
                        val p = request.url.parameters["challenge"]?.let { challenge ->
                            val c = Base64.getUrlDecoder().decode(challenge)
                            val host = "${request.url.host}:${request.url.port}"
                            when (answer) {
                                Answer.Server -> proof(SERVER, c, host, true)
                                Answer.OtherKey -> proof(OTHER, c, host, true)
                                Answer.NoProof -> null
                                Answer.NotLocal -> proof(SERVER, c, host, false)
                                Answer.OtherHost -> proof(SERVER, c, "10.9.9.9:8484", true)
                            }
                        }
                        json(ServerInfo.serializer(), ServerInfo("server-1", "Basement", "1.0", proof = p))
                    }
                    "/api/v1/users/me" -> json(CurrentUser.serializer(), anna)
                    else -> problem(HttpStatusCode.NotFound, "notFound")
                }
            }
        },
    )

    val network = NetworkEpoch()
    private val time = scope.testScheduler.timeSource
    val preferences = MemoryPreferences()

    /** A connection signed in, given the server's key when [key] (as a sign-in gives it), else none. */
    suspend fun connection(at: String, addresses: List<String>, key: String? = SERVER.encoded()): ServerConnection {
        val c = ServerConnection(
            "server-1", at, MemoryStore(), preferences, engine,
            addresses = addresses, background = EmptyCoroutineContext, pickTime = time, network = network,
        )
        c.signIn(TOKEN, key)
        return c
    }

    /** The requests to the local address that carried the token. */
    fun tokenAtLocal() = seen.filter { it.startsWith("192.168.1.20 ") && !it.endsWith(" -") }
}

class AddressTrustTest {
    @Test
    fun aPlainAddressThatProvesItselfCarriesTheToken() = runTest {
        val net = Net(this)
        val c = net.connection(LOCAL, listOf(LOCAL))

        assertEquals("anna", c.users.getCurrentUser().result().username)
        assertEquals(LOCAL, c.baseUrl)
        assertEquals(listOf("192.168.1.20 /api/v1/users/me Bearer $TOKEN"), net.tokenAtLocal())
        assertTrue(net.seen.first().startsWith("192.168.1.20 /api/v1/server -"), "proven first, without the token")
    }

    @Test
    fun anImpostorNeverGetsTheToken() = runTest {
        for (impostor in listOf(Answer.OtherKey, Answer.NoProof, Answer.NotLocal, Answer.OtherHost)) {
            val net = Net(this)
            net.answer = impostor
            val c = net.connection(LOCAL, listOf(LOCAL))

            assertFailsWith<UntrustedAddress>("$impostor") { c.users.getCurrentUser().result() }
            assertEquals(emptyList(), net.tokenAtLocal(), "$impostor")
            assertNull(c.authorization("$LOCAL/api/v1/images/i1?width=480"), "$impostor")
            assertNull(c.transferBase(), "$impostor")
        }
    }

    @Test
    fun anImpostorIsPassedOverForTheHttpsAddress() = runTest {
        val net = Net(this)
        net.answer = Answer.OtherKey
        val c = net.connection(LOCAL, listOf(LOCAL, PUBLIC))

        assertEquals("anna", c.users.getCurrentUser().result().username)
        assertEquals(PUBLIC, c.baseUrl)
        assertEquals(emptyList(), net.tokenAtLocal())
        assertTrue(c.signedIn.value, "the HTTPS address proves the server, so the session goes on")
    }

    @Test
    fun anAddressThatAnswersAsTheServerWithAnotherKeySignsOut() = runTest {
        val net = Net(this)
        net.answer = Answer.OtherKey
        val c = net.connection(LOCAL, listOf(LOCAL))

        assertFailsWith<UntrustedAddress> { c.users.getCurrentUser().result() }
        assertFalse(c.signedIn.value)
        assertTrue(c.identityChanged)
        assertEquals(emptyList(), net.tokenAtLocal())

        // Opening it signs out too, and finds it, to sign in to again.
        val opened = net.connection(LOCAL, listOf(LOCAL))
        assertEquals(LOCAL, assertNotNull(opened.reach()).first)
        assertFalse(opened.signedIn.value)
        assertTrue(opened.identityChanged)
        assertEquals(emptyList(), net.tokenAtLocal())

        // A new sign-in takes the key it gives.
        opened.signIn(TOKEN, OTHER.encoded())
        assertFalse(opened.identityChanged)
        assertEquals("anna", opened.users.getCurrentUser().result().username)
    }

    @Test
    fun signedInWithoutTheServersKeyTheFirstThatSignsIsKept() = runTest {
        val net = Net(this)
        val c = net.connection(LOCAL, listOf(LOCAL), key = null)

        assertEquals("anna", c.users.getCurrentUser().result().username)
        assertEquals(SERVER.encoded(), net.preferences.data.first()[stringPreferencesKey("kivo.serverKey.server-1")])

        // From then on it's that key's signature or nothing.
        net.network.changed()
        net.answer = Answer.OtherKey
        assertFailsWith<UntrustedAddress> { c.users.getCurrentUser().result() }
        assertTrue(c.identityChanged)
        assertEquals(1, net.tokenAtLocal().size)

        // It's kept across a restart, as a session restored there checks it.
        val restored = ServerConnection(
            "server-1", LOCAL, MemoryStore(mutableMapOf("kivo.token.server-1" to TOKEN)), net.preferences, net.engine,
            background = EmptyCoroutineContext, network = net.network,
        )
        assertTrue(restored.restore())
        net.answer = Answer.Server
        assertEquals(LOCAL, assertNotNull(restored.reach()).first)
        assertTrue(restored.signedIn.value)
    }

    @Test
    fun aNetworkChangeEndsTheTrustAtOnceAndRequestsGoToHttps() = runTest {
        val net = Net(this)
        val c = net.connection(LOCAL, listOf(LOCAL, PUBLIC))
        assertEquals(LOCAL, assertNotNull(c.reach()).first, "the local address proves itself, and wins")
        c.users.getCurrentUser().result()
        val image = c.imageUrl("i1", 480)
        assertEquals("$LOCAL/api/v1/images/i1?width=480", image)
        assertEquals("Bearer $TOKEN", c.authorization(image))
        assertEquals(PUBLIC, c.transferBase(), "downloads keep to the HTTPS address even at home")

        net.network.changed()

        // Before any request or probe: the local address carries nothing.
        assertNull(c.authorization(image), "a media or image URL made before isn't given the token")
        // The session moves off it at once, as it hears of the new network.
        c.address.untrust()
        assertEquals(PUBLIC, c.baseUrl)
        assertEquals("$PUBLIC/api/v1/images/i1?width=480", c.imageUrl("i1", 480))
        assertEquals("Bearer $TOKEN", c.authorization(c.imageUrl("i1", 480)))
        net.seen.clear()
        c.users.getCurrentUser().result()
        assertEquals(listOf("kivo.example.com /api/v1/users/me Bearer $TOKEN"), net.seen)

        // A pick proves the local address on this network, and it's home again.
        assertTrue(c.address.repick(AddressChange.Network))
        assertEquals(LOCAL, c.baseUrl)
        assertEquals("Bearer $TOKEN", c.authorization(image))
    }

    @Test
    fun withoutHttpsARequestWaitsForTheAddressToProveItselfAgain() = runTest {
        val net = Net(this)
        val c = net.connection(LOCAL, listOf(LOCAL))
        c.users.getCurrentUser().result()
        assertEquals(LOCAL, c.transferBase())

        net.network.changed()
        assertFalse(c.address.trustedNow())
        assertNull(c.transferBase(), "background transfers hold until it's proven")
        net.seen.clear()
        c.users.getCurrentUser().result()

        assertEquals(2, net.seen.size)
        assertTrue(net.seen[0].startsWith("192.168.1.20 /api/v1/server -"), "proven again first: ${net.seen}")
        assertEquals("192.168.1.20 /api/v1/users/me Bearer $TOKEN", net.seen[1])

        // On a network where an impostor holds the address, nothing goes.
        net.network.changed()
        net.answer = Answer.NotLocal
        net.seen.clear()
        assertFailsWith<UntrustedAddress> { c.users.getCurrentUser().result() }
        assertEquals(emptyList(), net.tokenAtLocal())
        assertTrue(c.signedIn.value, "an impostor that proves nothing doesn't sign out")
    }

    @Test
    fun theSessionOpensOnlyAtAnAddressThatProvesItself() = runTest {
        val net = Net(this)
        net.answer = Answer.NoProof
        val c = net.connection(LOCAL, listOf(LOCAL, PUBLIC))
        assertEquals(PUBLIC, assertNotNull(c.reach()).first)

        val alone = net.connection(LOCAL, listOf(LOCAL))
        assertNull(alone.reach())
        assertEquals(emptyList(), net.tokenAtLocal())
    }

    @Test
    fun ipv6LinkLocalAddressesArentKept() {
        val info = ServerInfo("server-1", "Basement", "1.0", localUrls = listOf("http://[fe80::1]:8484", LOCAL), publicUrl = PUBLIC)
        assertEquals(listOf(LOCAL, PUBLIC), SavedServer(info, "http://[fe9a::2]:8484").addresses)
        assertFalse(isLinkLocal("http://[fd00::1]:8484"))
    }

    @Test
    fun theHostMustBeTheOneProbed() {
        assertTrue(sameHost("192.168.1.20:8484", LOCAL))
        assertTrue(sameHost("NAS.local", "http://nas.local"))
        assertTrue(sameHost("[fd00::1]:8484", "http://[fd00::1]:8484"))
        assertFalse(sameHost("192.168.1.20", LOCAL))
        assertFalse(sameHost("192.168.1.21:8484", LOCAL))
        assertFalse(sameHost("[fd00::1]", "http://[fd00::1]:8484"))
    }
}
