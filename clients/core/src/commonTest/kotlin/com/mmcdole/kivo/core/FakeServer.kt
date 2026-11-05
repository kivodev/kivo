package com.mmcdole.kivo.core

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import com.mmcdole.kivo.api.infrastructure.ApiClient
import com.mmcdole.kivo.api.models.ServerProof
import com.mmcdole.kivo.core.auth.ClientIdentity
import com.mmcdole.kivo.core.auth.NetworkEpoch
import com.mmcdole.kivo.core.auth.ProbeKey
import com.mmcdole.kivo.core.auth.hmacSha256
import com.mmcdole.kivo.core.auth.message
import com.mmcdole.kivo.core.auth.sha256
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

/**
 * The proof a server holding a session of one of [tokens] answers a probe
 * [request] with, as reached at the probed host from a local network; null
 * for a probe of none of them, or a request that isn't a probe.
 */
internal fun proofOf(request: HttpRequestData, serverId: String, tokens: Collection<String>): ServerProof? =
    proofOf(request.url.parameters["challenge"], request.url.parameters["probeId"], "${request.url.host}:${request.url.port}", serverId, tokens)

/** The proof to a probe with [challenge] and [probeId] that reached [host]; null when it isn't one of [tokens]'s. */
internal fun proofOf(challenge: String?, probeId: String?, host: String, serverId: String, tokens: Collection<String>): ServerProof? {
    if (challenge == null || probeId == null) return null
    val token = tokens.firstOrNull { ProbeKey.of(it)?.probeId == probeId } ?: return null
    val base64 = Base64.UrlSafe.withPadding(Base64.PaddingOption.ABSENT)
    val mac = hmacSha256(sha256(base64.decode(token)), message(base64.decode(challenge), serverId, host, local = true))
    return ServerProof(ProbeKey.encode(mac), host, local = true)
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
