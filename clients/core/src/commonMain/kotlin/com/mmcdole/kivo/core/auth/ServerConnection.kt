package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.api.apis.DownloadsApi
import com.mmcdole.kivo.api.apis.FilesApi
import com.mmcdole.kivo.api.apis.LibrariesApi
import com.mmcdole.kivo.api.apis.PlaysApi
import com.mmcdole.kivo.api.apis.SessionsApi
import com.mmcdole.kivo.api.apis.SetupApi
import com.mmcdole.kivo.api.apis.UsersApi
import com.mmcdole.kivo.api.apis.WorksApi
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import com.mmcdole.kivo.api.models.CurrentUser
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.Reachability
import com.mmcdole.kivo.core.kivoJson
import com.mmcdole.kivo.core.resolveUrl
import io.ktor.client.HttpClient
import io.ktor.client.HttpClientConfig
import io.ktor.client.engine.HttpClientEngine
import io.ktor.client.plugins.HttpTimeout
import io.ktor.client.plugins.HttpTimeoutConfig
import io.ktor.client.plugins.api.createClientPlugin
import io.ktor.client.plugins.contentnegotiation.ContentNegotiation
import io.ktor.client.plugins.timeout
import io.ktor.client.plugins.websocket.WebSockets
import io.ktor.client.statement.HttpResponsePipeline
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpMethod
import io.ktor.http.HttpStatusCode
import io.ktor.http.URLBuilder
import io.ktor.http.encodedPath
import io.ktor.serialization.kotlinx.json.json
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerializationException
import kotlin.coroutines.CoroutineContext
import kotlin.time.Duration
import kotlin.time.TimeSource
import kotlin.time.Duration.Companion.seconds

/**
 * One server, reached at [baseUrl], with this app's session on it. The
 * session token is kept in the [SecureStore] under the server's ID, so a
 * changed address doesn't sign the app out, and goes in the `Authorization`
 * header of every request, the live-events upgrade too, never in a URL. A
 * `401` to a request that carried it signs the app out, as does the
 * live-events socket closing with 4401; a wrong current password to
 * change-password doesn't.
 *
 * Who the session is for ([user]) is kept beside the token, in the app's
 * [preferences] under the server's ID, so it shows without the server; it's
 * written and cleared with the token.
 *
 * The HTTP client is the platform's (OkHttp on Android, Darwin on Apple)
 * unless [engine] is given, as tests give Ktor's mock engine; the address
 * probes go through [probes], the app's one ([probeClient]). Its requests
 * wait for the server as [timeouts] say, so one to a server that doesn't
 * answer fails in seconds, and their answers are decoded in the
 * [background], so its suspend calls are safe from the main thread.
 * Whether the server answers its reads, and when they ask again, is its
 * [reachability].
 *
 * The server is reached at one of its [addresses] at a time ([address]),
 * picked again as the device's situation changes; a read that gets no
 * answer picks again before it waits to ask again. Every request goes to
 * the address in use as it's sent, however it was made.
 *
 * Every request says which app sends it, on which device ([identity]), in
 * its identity headers.
 *
 * The token goes only where [address] trusts it now ([ServerAddress.carries]):
 * a request that would carry it waits for a trusted address, and isn't
 * sent when there's none; [authorization] gives it for a URL only at one.
 * Its probes prove a plain-HTTP address, on the device's [network], by the
 * server's signature with the [ServerKey] it gave at sign-in; with no key
 * kept, only HTTPS addresses are trusted. The key is kept in the
 * [preferences] under the server's ID, and a new sign-in replaces it.
 */
internal class ServerConnection(
    val serverId: String,
    reachedAt: String,
    private val store: SecureStore,
    private val preferences: DataStore<Preferences>,
    engine: HttpClientEngine? = null,
    private val timeouts: ServerTimeouts = ServerTimeouts(),
    addresses: List<String> = listOf(reachedAt),
    private val background: CoroutineContext = Dispatchers.Default,
    pickTime: TimeSource = TimeSource.Monotonic,
    private val network: NetworkEpoch = NetworkEpoch(),
    private val identity: ClientIdentity? = null,
    private val probes: HttpClient = probeClient(engine, identity),
) {
    private val tokenKey = tokenKey(serverId)
    private val token = MutableStateFlow<String?>(null)
    private val mutableSignedIn = MutableStateFlow(false)
    private val userKey = stringPreferencesKey("kivo.user.$serverId")
    private val mutableUser = MutableStateFlow<SignedInUser?>(null)
    private val serverKeyKey = stringPreferencesKey("kivo.serverKey.$serverId")
    private val serverKey = MutableStateFlow<ServerKey?>(null)

    /** Orders keeping the user against signing in and out, so a late answer can't outlive the session. */
    private val lock = Mutex()

    /** Whether this app holds a token for the server. */
    val signedIn: StateFlow<Boolean> = mutableSignedIn.asStateFlow()

    /**
     * Who this app is signed in as, from the server's last answer to
     * `/users/me`; null when signed out, or until the server first says.
     */
    val user: StateFlow<SignedInUser?> = mutableUser.asStateFlow()

    /** The identity headers every request carries; none without an identity, as in tests. */
    private val identityHeaders: Map<String, String> = identity?.let(::identityHeaders).orEmpty()

    /** The `Authorization` header this app sends; null when signed out. */
    private val bearer: String?
        get() = token.value?.let { "Bearer $it" }

    /**
     * The `Authorization` header for a request a platform makes outside
     * [http] to [url], such as its image loader's: null when signed out, or
     * when [url] isn't at an address the token may go to now.
     */
    fun authorization(url: String): String? {
        val header = bearer ?: return null
        val at = try {
            URLBuilder(url)
        } catch (_: Exception) {
            return null
        }
        return header.takeIf { address.carries(at) }
    }


    /** Where the server is reached now, among its addresses. */
    val address = ServerAddress(serverId, reachedAt, addresses, ::probe, network, time = pickTime, context = background)

    /**
     * What [at] says it is, when it may be trusted with the token: an HTTPS
     * address that answers, or, signed in, a plain-HTTP one whose proof to
     * the probe's challenge the server's key signed ([ServerKey.signed]),
     * that says the probe came from a local network and names the host
     * probed; signed out, any that answers, as there's no token to keep
     * from it.
     */
    private suspend fun probe(at: String): ServerInfo? {
        val api = SetupApi(at, probes)
        if (token.value == null || isSecure(at)) return api.getServer().result()
        val key = serverKey.value ?: return null
        val (challenge, sent) = ServerKey.challenge()
        val info = api.getServer(challenge = sent).result()
        if (info.id != serverId) return null
        if (!key.signed(info.proof, challenge, serverId)) return null
        val proof = info.proof ?: return null
        if (!proof.local || !sameHost(proof.host, at)) return null
        return info
    }

    private suspend fun keepKey(key: ServerKey?) {
        preferences.edit { if (key == null) it.remove(serverKeyKey) else it[serverKeyKey] = key.encoded }
        serverKey.value = key
    }

    /** The base URL the server is reached at now. */
    val baseUrl: String get() = address.current.value

    /**
     * The base URL of the address background transfers fetch from: the
     * server's HTTPS one when it has one, so a transfer that goes on while
     * the app is away never carries the token to an address unproven where
     * the device is then; else the address in use while it may be trusted
     * with the token; null while it may not.
     */
    fun transferBase(): String? = address.secure() ?: baseUrl.takeIf { address.trustedNow() }

    /** Whether the server answers, and the reads of it asking again while it doesn't, once the address is picked again. */
    val reachability = Reachability { address.repick(AddressChange.NoAnswer) }

    val http: HttpClient = (engine?.let { HttpClient(it, ::configure) } ?: HttpClient(::configure)).apply {
        // An answer is decoded in the background, so a call from the main thread doesn't hold it while a long list is read.
        responsePipeline.intercept(HttpResponsePipeline.Receive) { withContext(background) { proceed() } }
    }

    val sessions = SessionsApi(reachedAt, http)
    val setup = SetupApi(reachedAt, http)
    val users = UsersApi(reachedAt, http)
    val libraries = LibrariesApi(reachedAt, http)
    val works = WorksApi(reachedAt, http)
    val plays = PlaysApi(reachedAt, http)
    val downloads = DownloadsApi(reachedAt, http)
    val files = FilesApi(reachedAt, http)

    /** [relative], a URL the API gives relative to its root (`<baseUrl>/api/v1/`), made absolute; an absolute one stays as it is. */
    fun apiUrl(relative: String): String = apiUrl(baseUrl, relative)

    /** [relative] made absolute at [base]'s API root. */
    fun apiUrl(base: String, relative: String): String = resolveUrl(base.trimEnd('/') + "/api/v1/", relative)

    /**
     * Picks the address to reach the server at as the session opens, among
     * all of them: the address and what it said of itself; null when none
     * answers as it, or none may be trusted with the token.
     */
    suspend fun reach(): Pair<String, ServerInfo>? = address.open()

    /** An image by ID at a width in pixels; the server serves the smallest stored width at least that. */
    fun imageUrl(id: String, width: Int): String = apiUrl(imagePath(id, width))

    /** Reads the stored token, who it's for and the server's key; whether there is a token. Its first `401` signs out. */
    suspend fun restore(): Boolean = lock.withLock {
        serverKey.value = ServerKey.of(preferences.data.first()[serverKeyKey])
        setToken(store.read(tokenKey))
        mutableUser.value = token.value?.let { storedUser() }
        token.value != null
    }

    /**
     * Keeps the token of the session just signed in, and the server's [key]
     * in place of any before; who it's for is kept once the server says.
     */
    internal suspend fun signIn(sessionToken: String, key: String? = null) = lock.withLock {
        store.write(tokenKey, sessionToken)
        keepKey(ServerKey.of(key))
        forgetUser()
        setToken(sessionToken)
    }

    /** Keeps [current] as who this app is signed in as, while it is. */
    internal suspend fun keep(current: CurrentUser) = lock.withLock {
        val user = SignedInUser(current)
        if (!signedIn.value || user == mutableUser.value) return@withLock
        preferences.edit { it[userKey] = kivoJson.encodeToString(SignedInUser.serializer(), user) }
        mutableUser.value = user
    }

    /** Ends this app's session on the server, where it can be reached, then forgets the token as [signOut] does. */
    suspend fun endSession() {
        try {
            sessions.deleteCurrentSession()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // Signed out here anyway; "Signed-in devices" can still revoke the session.
        }
        signOut()
    }

    /** Forgets the token and who it was for: the server said it's no longer valid, or the user signed out. */
    suspend fun signOut() = lock.withLock {
        store.delete(tokenKey)
        forgetUser()
        setToken(null)
    }

    private suspend fun storedUser(): SignedInUser? {
        val stored = preferences.data.first()[userKey] ?: return null
        return try {
            kivoJson.decodeFromString(SignedInUser.serializer(), stored)
        } catch (_: SerializationException) {
            null
        }
    }

    private suspend fun forgetUser() {
        preferences.edit { it.remove(userKey) }
        mutableUser.value = null
    }

    private fun setToken(value: String?) {
        if (value == token.value) return
        token.value = value
        address.reset()
        mutableSignedIn.value = value != null
    }

    companion object {
        private fun tokenKey(serverId: String) = "kivo.token.$serverId"

        /** Whether [store] holds this app's session token for [serverId]. */
        suspend fun holdsSession(store: SecureStore, serverId: String): Boolean = store.read(tokenKey(serverId)) != null
    }

    private fun configure(config: HttpClientConfig<*>) {
        config.install(ContentNegotiation) { json(kivoJson) }
        config.install(WebSockets)
        config.install(identified(identityHeaders))
        config.install(HttpTimeout) {
            connectTimeoutMillis = timeouts.connect.inWholeMilliseconds
            socketTimeoutMillis = timeouts.idle.inWholeMilliseconds
            requestTimeoutMillis = timeouts.request.inWholeMilliseconds
        }
        config.install(
            createClientPlugin("KivoBearer") {
                onRequest { request, _ ->
                    val header = bearer
                    // A request to the server that would carry the token waits for an address that may be trusted with it.
                    val ours = address.knows(request.url)
                    if (header != null && ours) address.trusted()
                    address.rebase(request.url)
                    timeouts.waitOf(request.method, request.url.encodedPath)?.let { wait ->
                        request.timeout {
                            requestTimeoutMillis = wait.whole?.inWholeMilliseconds ?: HttpTimeoutConfig.INFINITE_TIMEOUT_MS
                            socketTimeoutMillis = wait.idle?.inWholeMilliseconds ?: HttpTimeoutConfig.INFINITE_TIMEOUT_MS
                        }
                    }
                    if (header != null && ours) {
                        // The network may have changed since: it goes nowhere the token may not.
                        if (!address.carries(request.url)) throw UntrustedAddress()
                        request.headers[HttpHeaders.Authorization] = header
                    }
                }
                // Only a 401 to the token held now: a late one to a replaced token, or to a
                // request without one (a wrong password), signs nothing out, and nor does a
                // wrong current password to change-password.
                onResponse { response ->
                    val sent = response.call.request.headers[HttpHeaders.Authorization] ?: return@onResponse
                    if (response.status != HttpStatusCode.Unauthorized || sent != bearer) return@onResponse
                    if (response.call.request.url.encodedPath.endsWith(CHANGE_PASSWORD)) return@onResponse
                    signOut()
                }
            },
        )
    }
}

/**
 * How long a request waits for the server: [connect] to reach it, [idle]
 * with nothing arriving, and [request] for the whole answer. Apple's client
 * has no connect timeout of its own, so there [idle] covers connecting too.
 *
 * Starting a play and its fallback, which the server answers once the
 * play's transcode has its first segment, wait [slow], whole and idle.
 * Media (artwork, fonts, a play's or a download's files) waits [slow] idle,
 * with no limit on the whole, as a large file may take longer. The live
 * events' socket has neither, as it's quiet until something happens.
 * [connect] holds for every request.
 */
internal data class ServerTimeouts(
    val connect: Duration = 5.seconds,
    val idle: Duration = 5.seconds,
    val request: Duration = 15.seconds,
    val slow: Duration = 60.seconds,
) {
    /** A request's own waits; null for no limit. */
    internal data class Wait(val whole: Duration?, val idle: Duration?)

    /** The waits of a request that doesn't take the usual ones, by its method and path; null for one that does. */
    internal fun waitOf(method: HttpMethod, path: String): Wait? {
        if (API !in path) return null
        val parts = path.substringAfter(API).split('/')
        return when {
            parts == listOf("events") -> Wait(null, null)
            method == HttpMethod.Post && parts == listOf("plays") -> Wait(slow, slow)
            method == HttpMethod.Post && parts.size == 3 && parts[0] == "plays" && parts[2] == "fallback" -> Wait(slow, slow)
            method == HttpMethod.Get && parts[0] in OWN_MEDIA -> Wait(null, slow)
            method == HttpMethod.Get && parts[0] in PLAY_MEDIA && parts.size >= 4 -> Wait(null, slow)
            else -> null
        }
    }

    private companion object {
        const val API = "/api/v1/"

        /** Media by its own ID: `images/{id}` and `fonts/{hash}`. */
        val OWN_MEDIA = setOf("images", "fonts")

        /** Media of a play or a download: `plays/{id}/{kind}/{name}` and `downloads/{id}/{kind}/{name}`. */
        val PLAY_MEDIA = setOf("plays", "downloads")
    }
}

/**
 * The client that asks an address who's there (`GET /api/v1/server`),
 * outside a session's client, which would send it to the address in use:
 * one for the app, which finding servers and every connection's probes
 * share. Each request waits [PICK_WAIT] at most and says which app sends it
 * ([identity]).
 */
internal fun probeClient(engine: HttpClientEngine?, identity: ClientIdentity?): HttpClient {
    val headers = identity?.let(::identityHeaders).orEmpty()
    val configure: HttpClientConfig<*>.() -> Unit = {
        install(ContentNegotiation) { json(kivoJson) }
        install(HttpTimeout) { requestTimeoutMillis = PICK_WAIT.inWholeMilliseconds }
        install(identified(headers))
    }
    return engine?.let { HttpClient(it, configure) } ?: HttpClient(configure)
}

/** Puts the identity [headers] on every request. */
private fun identified(headers: Map<String, String>) = createClientPlugin("KivoIdentity") {
    onRequest { request, _ ->
        for ((name, value) in headers) request.headers[name] = value
    }
}

/** Changing the user's own password, whose `401` is a wrong current password, not an ended session. */
private const val CHANGE_PASSWORD = "/api/v1/users/me/change-password"

/** An image by ID at a width in pixels, relative to the API's root. */
internal fun imagePath(id: String, width: Int): String = "images/$id?width=$width"
