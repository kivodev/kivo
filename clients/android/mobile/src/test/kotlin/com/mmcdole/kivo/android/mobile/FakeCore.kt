package com.mmcdole.kivo.android.mobile

import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import com.mmcdole.kivo.api.models.CurrentUser
import com.mmcdole.kivo.api.models.PlaybackPreferences
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.api.models.Session
import com.mmcdole.kivo.api.models.SubtitleMode
import com.mmcdole.kivo.api.models.UserLimits
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.auth.ClientIdentity
import com.mmcdole.kivo.core.auth.SecureStore
import com.mmcdole.kivo.core.caps.AudioSink
import com.mmcdole.kivo.core.caps.CapabilityProbe
import com.mmcdole.kivo.core.caps.Measured
import com.mmcdole.kivo.core.discovery.FoundServer
import com.mmcdole.kivo.core.downloads.DownloadTransport
import app.cash.sqldelight.driver.jdbc.sqlite.JdbcSqliteDriver
import com.mmcdole.kivo.core.session.Device
import com.mmcdole.kivo.core.session.KivoClient
import com.mmcdole.kivo.core.session.KivoSession
import com.mmcdole.kivo.core.session.androidDownloads
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.engine.mock.respond
import io.ktor.client.request.HttpRequestData
import io.ktor.client.request.HttpResponseData
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.ViewModelStore
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.newSingleThreadContext
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.setMain
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.KSerializer
import kotlinx.serialization.json.Json
import java.io.File
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.io.path.createTempDirectory
import kotlin.time.Instant

// The core as the screens' view models get it, against a server Ktor's mock
// engine answers by method and path: a KivoClient on a device with its
// preferences in a temporary folder, signed in over HTTPS.

private val json = Json { encodeDefaults = false; explicitNulls = false }

val anna = CurrentUser("u1", "anna", admin = false, limits = UserLimits(), playback = PlaybackPreferences(SubtitleMode.Smart, true, true, 1.0))

/** A work's summary with no artwork. */
fun work(id: String, title: String = id, kind: String = "movie") =
    WorkSummary(id = id, kind = kind, title = title, matchState = "matched", available = true, images = WorkImages())

class FakeServer {
    private val routes = mutableMapOf<String, suspend MockRequestHandleScope.(HttpRequestData) -> HttpResponseData>()

    /** Every request seen, as "METHOD path?query". */
    val seen: MutableList<String> = CopyOnWriteArrayList()

    val engine = MockEngine { request ->
        val key = "${request.method.value} ${request.url.encodedPath}"
        seen += key + (request.url.encodedQuery.takeIf { it.isNotEmpty() }?.let { "?$it" } ?: "")
        routes[key]?.invoke(this, request)
            ?: refuse(HttpStatusCode.NotFound, "notFound")
    }

    fun on(method: String, path: String, answer: suspend MockRequestHandleScope.(HttpRequestData) -> HttpResponseData) {
        routes["$method $path"] = answer
    }

    fun <T> get(path: String, serializer: KSerializer<T>, value: () -> T) = on("GET", path) { reply(serializer, value()) }

    init {
        get("/api/v1/server", ServerInfo.serializer()) { ServerInfo("s1", "Den", "1.0") }
        get("/api/v1/users/me", CurrentUser.serializer()) { anna }
        on("POST", "/api/v1/sessions") {
            val now = Instant.fromEpochMilliseconds(0)
            reply(Session.serializer(), Session("x", "app", "Phone", now, now, true, token = "token"), HttpStatusCode.Created)
        }
    }
}

fun <T> MockRequestHandleScope.reply(serializer: KSerializer<T>, value: T, status: HttpStatusCode = HttpStatusCode.OK) =
    respond(json.encodeToString(serializer, value), status, headersOf(HttpHeaders.ContentType, "application/json"))

/** A problem document with [code]. */
fun MockRequestHandleScope.refuse(status: HttpStatusCode, code: String) = respond(
    """{"type":"about:blank","title":"${status.description}","status":${status.value},"code":"$code","requestId":"r"}""",
    status,
    headersOf(HttpHeaders.ContentType, "application/problem+json"),
)

private class Memory : SecureStore {
    private val values = mutableMapOf<String, String>()

    override suspend fun read(key: String) = values[key]

    override suspend fun write(key: String, value: String) {
        values[key] = value
    }

    override suspend fun delete(key: String) {
        values.remove(key)
    }
}

/** A phone in a test, its app at [version]; with [transport], one that keeps downloads, its store in SQLite in memory and its files in [dir]. */
class TestDevice(dir: File, transport: DownloadTransport? = null, version: String = "test") : Device {
    override val name = "Test phone"
    override val identity = object : ClientIdentity {
        override val app = "kivo-android"
        override val appVersion = version
        override val platform = "androidPhone"
        override val osVersion = "17"
        override val deviceModel = "test"
    }
    override val preferences = PreferenceDataStoreFactory.create { File(dir, "device.preferences_pb") }
    override val secureStore: SecureStore = Memory()
    override val capabilities = object : CapabilityProbe {
        override suspend fun measure() = Measured(containers = listOf("mp4"), hls = true, video = emptyList(), audio = emptyList(), subtitles = emptyList())

        override fun audioSink() = AudioSink()
    }
    override val downloads = transport?.let {
        androidDownloads(File(dir, "downloads").path, it) { schema -> JdbcSqliteDriver(JdbcSqliteDriver.IN_MEMORY).also { driver -> schema.create(driver) } }
    }
}

/** The address the fake server answers at; HTTPS, so the session's token goes to it without a probe. */
const val SERVER = "https://kivo.test"

/**
 * A test of the core and a view model over it: the main thread is one of
 * its own, as the app's is, and [signIn] opens a session on [server].
 */
@OptIn(ExperimentalCoroutinesApi::class)
open class CoreTest {
    val server = FakeServer()
    private val dir = createTempDirectory("mobile-test").toFile()
    private val main = newSingleThreadContext("main")

    init {
        Dispatchers.setMain(main)
    }

    /** The device's download transfers, for a test of downloads; none otherwise, as on a TV. */
    open val transport: DownloadTransport? get() = null

    /** The app's version, as the device says it. */
    open val appVersion: String get() = "test"

    /** Where the device keeps its preferences and downloads. */
    val folder: File get() = dir

    val client: KivoClient by lazy { KivoClient(TestDevice(dir, transport, appVersion), server.engine) }

    fun signIn(): KivoSession = runBlocking { client.signIn(FoundServer(SERVER, ServerInfo("s1", "Den", "1.0")), "anna", "pw", "Phone") }
        .also { sessions += it }

    private val sessions = mutableListOf<KivoSession>()
    private val viewModels = ViewModelStore()

    /** [make]'s view model, cleared as the test ends, as a screen's is as it goes: for one whose work outlives a test otherwise, such as a play. */
    inline fun <reified T : ViewModel> held(noinline make: () -> T): T =
        ViewModelProvider.create(heldStore(), viewModelFactory { initializer { make() } })[T::class]

    fun heldStore(): ViewModelStore = viewModels

    @org.junit.After
    fun tearDown() {
        // What the test started stops on the main thread before the next test takes it.
        runBlocking(Dispatchers.Main) {
            viewModels.clear()
            sessions.forEach { it.close() }
            delay(200)
        }
        Dispatchers.resetMain()
        main.close()
        dir.deleteRecursively()
    }
}

/** Returns once [ok] holds, as of something the test records, asking again every few milliseconds for a few seconds. */
fun waitFor(ok: () -> Boolean) = runBlocking {
    withTimeout(5_000) {
        while (!ok()) delay(10)
    }
}

/** The first value of [flow] that [ok] accepts, within a few seconds. */
fun <T> awaitValue(flow: Flow<T>, ok: (T) -> Boolean): T = runBlocking { withTimeout(5_000) { flow.first(ok) } }
