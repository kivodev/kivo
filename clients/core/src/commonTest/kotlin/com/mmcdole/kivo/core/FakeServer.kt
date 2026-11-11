package com.mmcdole.kivo.core

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import com.mmcdole.kivo.api.infrastructure.ApiClient
import com.mmcdole.kivo.api.models.ServerProof
import com.mmcdole.kivo.core.auth.ClientIdentity
import com.mmcdole.kivo.core.auth.NetworkEpoch
import com.mmcdole.kivo.core.auth.encode
import com.mmcdole.kivo.core.auth.message
import com.mmcdole.kivo.core.auth.modL
import com.mmcdole.kivo.core.auth.reduce
import com.mmcdole.kivo.core.auth.scalarBase
import com.mmcdole.kivo.core.auth.sha512
import com.mmcdole.kivo.core.auth.SecureStore
import com.mmcdole.kivo.core.auth.ServerConnection
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockEngineConfig
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.respond
import io.ktor.client.request.HttpRequestData
import io.ktor.client.request.HttpResponseData
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.content.OutgoingContent
import io.ktor.http.headersOf
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.KSerializer
import kotlin.coroutines.EmptyCoroutineContext
import kotlin.io.encoding.Base64
import kotlin.time.TimeSource

/** A request the fake server saw. */
data class Seen(
    val method: String,
    val path: String,
    val query: String,
    val body: String,
    val authorization: String?,
    /** The identity headers, by name. */
    val identity: Map<String, String> = emptyMap(),
)

/**
 * The server, as canned answers by method and path over Ktor's mock engine.
 * An answer may suspend, to hold a response back until a test lets it go.
 * Anything without an answer is a 404 problem.
 */
class FakeServer(dispatcher: CoroutineDispatcher? = null) {
    val seen = mutableListOf<Seen>()
    private val routes = mutableMapOf<String, suspend MockRequestHandleScope.(HttpRequestData) -> HttpResponseData>()

    /** The mock engine, on [dispatcher] when given, so a test's virtual time runs the requests too. */
    val engine = MockEngine(
        MockEngineConfig().apply {
            this.dispatcher = dispatcher
            addHandler { request ->
                val body = (request.body as? OutgoingContent.ByteArrayContent)?.bytes()?.decodeToString() ?: ""
                val path = request.url.encodedPath
                seen += Seen(
                    request.method.value,
                    path,
                    request.url.encodedQuery,
                    body,
                    request.headers[HttpHeaders.Authorization],
                    request.headers.entries().filter { it.key.startsWith("Kivo-", ignoreCase = true) }.associate { it.key to it.value.first() },
                )
                val route = routes["${request.method.value} $path"]
                route?.invoke(this, request) ?: problem(HttpStatusCode.NotFound, "notFound")
            }
        },
    )

    fun on(method: String, path: String, answer: suspend MockRequestHandleScope.(HttpRequestData) -> HttpResponseData) {
        routes["$method $path"] = answer
    }

    /** The requests to [path], by method. */
    fun to(method: String, path: String) = seen.filter { it.method == method && it.path == path }

    /** A connection to the server at `https://kivo.test`, whose address picks run on the test's clock, as the engine does. */
    internal fun connection(
        store: SecureStore = MemoryStore(),
        preferences: MemoryPreferences = MemoryPreferences(),
        at: String = "https://kivo.test",
        addresses: List<String> = listOf(at),
        network: NetworkEpoch = NetworkEpoch(),
        pickTime: TimeSource = TimeSource.Monotonic,
        serverId: String = "server-1",
        identity: ClientIdentity? = null,
    ) = ServerConnection(
        serverId, at, store, preferences, engine,
        addresses = addresses, background = EmptyCoroutineContext, network = network, pickTime = pickTime, identity = identity,
    )
}

fun <T> MockRequestHandleScope.json(serializer: KSerializer<T>, value: T, status: HttpStatusCode = HttpStatusCode.OK) =
    respond(
        ApiClient.JSON_DEFAULT.encodeToString(serializer, value),
        status,
        headersOf(HttpHeaders.ContentType, "application/json"),
    )

fun MockRequestHandleScope.problem(
    status: HttpStatusCode,
    code: String,
    extra: String = "",
) = respond(
    """{"type":"about:blank","title":"${status.description}","status":${status.value},"code":"$code","requestId":"r"$extra}""",
    status,
    headersOf(HttpHeaders.ContentType, "application/problem+json"),
)

fun MockRequestHandleScope.empty(status: HttpStatusCode = HttpStatusCode.NoContent) = respond("", status)

private val base64url = Base64.UrlSafe.withPadding(Base64.PaddingOption.ABSENT)

/** A server's Ed25519 identity key, from its 32-byte [seed], which signs as RFC 8032 says. */
internal class TestServerKey(seed: ByteArray) {
    private val hashed = sha512(seed)
    private val scalar = hashed.copyOf(32).also {
        it[0] = (it[0].toInt() and 248).toByte()
        it[31] = (it[31].toInt() and 127 or 64).toByte()
    }
    private val public = encode(scalarBase(scalar))

    /** The public key, as the server gives it. */
    val encoded: String = base64url.encode(public)

    fun sign(message: ByteArray): ByteArray {
        val r = sha512(hashed.copyOfRange(32, 64) + message).also(::reduce)
        val big = encode(scalarBase(r))
        val h = sha512(big + public + message).also(::reduce)
        val x = LongArray(64)
        for (i in 0 until 32) x[i] = r[i].toLong() and 0xff
        for (i in 0 until 32) for (j in 0 until 32) x[i + j] += (h[i].toLong() and 0xff) * (scalar[j].toLong() and 0xff)
        return big + ByteArray(32).also { modL(it, x) }
    }
}

/** The test server's identity key. */
internal val SERVER_KEY = TestServerKey(ByteArray(32) { it.toByte() })

/**
 * The proof a server with [key] answers a probe [request] with, as reached
 * at the probed host from a local network; null for a request that isn't a
 * probe.
 */
internal fun proofOf(request: HttpRequestData, serverId: String, key: TestServerKey = SERVER_KEY): ServerProof? =
    proofOf(request.url.parameters["challenge"], "${request.url.host}:${request.url.port}", serverId, key)

/** The proof to a probe with [challenge] that reached [host]; null without a challenge. */
internal fun proofOf(challenge: String?, host: String, serverId: String, key: TestServerKey = SERVER_KEY): ServerProof? {
    if (challenge == null) return null
    val signature = key.sign(message(base64url.decode(challenge), serverId, host, local = true))
    return ServerProof(base64url.encode(signature), key.encoded, host, local = true)
}

/** A [SecureStore] in memory. */
class MemoryStore(val values: MutableMap<String, String> = mutableMapOf()) : SecureStore {
    override suspend fun read(key: String) = values[key]

    override suspend fun write(key: String, value: String) {
        values[key] = value
    }

    override suspend fun delete(key: String) {
        values.remove(key)
    }
}

/** A preferences [DataStore] in memory. */
class MemoryPreferences : DataStore<Preferences> {
    private val lock = Mutex()
    private val state = MutableStateFlow(emptyPreferences())

    override val data: StateFlow<Preferences> = state

    override suspend fun updateData(transform: suspend (t: Preferences) -> Preferences): Preferences =
        lock.withLock { transform(state.value).also { state.value = it } }
}
