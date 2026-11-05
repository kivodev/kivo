package com.mmcdole.kivo.core.session

import app.cash.sqldelight.driver.jdbc.sqlite.JdbcSqliteDriver
import com.mmcdole.kivo.api.models.CastToken
import com.mmcdole.kivo.api.models.CurrentUser
import com.mmcdole.kivo.api.models.DownloadEstimateList
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryList
import com.mmcdole.kivo.api.models.PlaybackPreferences
import com.mmcdole.kivo.api.models.QualityRequest
import com.mmcdole.kivo.api.models.QualityRung
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.api.models.Session
import com.mmcdole.kivo.api.models.UserLimits
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkImages
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.api.models.WorkSummaryPage
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.MemoryPreferences
import com.mmcdole.kivo.core.MemoryStore
import com.mmcdole.kivo.core.auth.ClientIdentity
import com.mmcdole.kivo.core.auth.SavedServer
import com.mmcdole.kivo.core.caps.AudioSink
import com.mmcdole.kivo.core.caps.CapabilityProbe
import com.mmcdole.kivo.core.caps.Measured
import com.mmcdole.kivo.core.data.GridQuery
import com.mmcdole.kivo.core.discovery.FoundServer
import com.mmcdole.kivo.core.downloads.DownloadDisk
import com.mmcdole.kivo.core.downloads.DownloadTransport
import com.mmcdole.kivo.core.downloads.TransferRequest
import com.mmcdole.kivo.core.downloads.db.DownloadDatabase
import com.mmcdole.kivo.core.empty
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.play.FakeEngine
import com.mmcdole.kivo.core.player.Autoplay
import com.mmcdole.kivo.core.player.UpNextPhase
import com.mmcdole.kivo.core.problem
import com.mmcdole.kivo.core.proofOf
import com.mmcdole.kivo.core.settings.Quality
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandleScope
import io.ktor.client.request.HttpRequestData
import io.ktor.client.request.HttpResponseData
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import java.io.File
import java.io.IOException
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.io.path.createTempDirectory
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlin.time.Duration.Companion.seconds
import kotlin.time.Instant

// The session graph as the apps get it from KivoClient: opening a saved
// server, signing in and out, moving to another server, and the plays it
// starts with this device's settings, against servers answered by Ktor's
// mock engine by host.

private val ladder = listOf(QualityRung(20_000_000, 1080), QualityRung(8_000_000, 720))

private fun info(id: String, name: String = id, localUrls: List<String>? = null) =
    ServerInfo(id, name, "1.0", localUrls = localUrls, qualityLadder = ladder)

private val anna = CurrentUser("u1", "anna", admin = false, limits = UserLimits(), playback = PlaybackPreferences("smart", true, true, 1.0))

/** Servers by host: what each answers, by method and path, and every request they saw as "host METHOD path". */
private class Servers {
    val seen: MutableList<String> = CopyOnWriteArrayList()

    /** Each request's app and app version headers, in order. */
    val clients: MutableList<String?> = CopyOnWriteArrayList()
    private val routes = mutableMapOf<String, suspend MockRequestHandleScope.(HttpRequestData) -> HttpResponseData>()

    val engine = MockEngine { request ->
        val key = "${request.url.host} ${request.method.value} ${request.url.encodedPath}"
        seen += key
        clients += request.headers["Kivo-App"]?.let { "$it ${request.headers["Kivo-App-Version"]}" }
        routes[key]?.invoke(this, request) ?: problem(HttpStatusCode.NotFound, "notFound")
    }

    fun on(host: String, method: String, path: String, answer: suspend MockRequestHandleScope.(HttpRequestData) -> HttpResponseData) {
        routes["$host $method $path"] = answer
    }

    /** The session tokens the servers hold, to prove themselves to a probe with. */
    var tokens: () -> Collection<String> = { emptyList() }

    /** A server at [host] that says it's [info], and who the session is for. */
    fun server(host: String, info: ServerInfo) {
        on(host, "GET", "/api/v1/server") { json(ServerInfo.serializer(), info.copy(proof = proofOf(it, info.id, tokens()))) }
        on(host, "GET", "/api/v1/users/me") { json(CurrentUser.serializer(), anna) }
    }
}

private class Probe : CapabilityProbe {
    override suspend fun measure() = Measured(containers = listOf("mp4"), hls = true, video = emptyList(), audio = emptyList(), subtitles = emptyList())

    override fun audioSink() = AudioSink()
}

private class Transport : DownloadTransport {
    override fun start(request: TransferRequest) = Unit

    override fun stop(id: String) = Unit

    override fun cancel(id: String) = Unit

    override fun running() = emptyList<String>()
}

private class Disk(override val folder: String) : DownloadDisk {
    override fun freeBytes() = Long.MAX_VALUE

    override fun sha256(path: String) = ""
}

private object Identity : ClientIdentity {
    override val app = "kivo-android"
    override val appVersion = "1.0"
    override val platform = "androidTv"
    override val osVersion = "14"
    override val deviceModel = "SHIELD Android TV"
}

private class TestDevice(withDownloads: Boolean) : Device {
    override val name = "Living Room"
    override val identity = Identity
    override val language = "en"
    override val preferences = MemoryPreferences()
    override val secureStore = MemoryStore()
    override val capabilities = Probe()
    override val downloads: DownloadDevice? = if (!withDownloads) {
        null
    } else {
        val dir = createTempDirectory("kivo-session").toFile()
        val driver = JdbcSqliteDriver("jdbc:sqlite:${File(dir, "downloads.db").path}").also { DownloadDatabase.Schema.create(it) }
        DownloadDevice(driver, Disk(File(dir, "downloads").path), Transport())
    }

    /** Holds a session token for [serverId], as an app signed in before does. */
    fun signedInTo(serverId: String) {
        secureStore.values["kivo.token.$serverId"] = java.util.Base64.getUrlEncoder().withoutPadding().encodeToString("token-$serverId".toByteArray())
    }
}

private class Harness(scope: TestScope, withDownloads: Boolean = false) {
    val servers = Servers()
    val device = TestDevice(withDownloads)
    val core = KivoClient(device, servers.engine, scope.backgroundScope)

    init {
        servers.tokens = { device.secureStore.values.values }
    }
}

class KivoSessionTest {
    @Test
    fun restoringOpensTheSavedServerWhereItAnswersAndKeepsItsAddresses() = runTest {
        val h = Harness(this)
        h.servers.server("home.test", info("server-1", "Basement", localUrls = listOf("http://home.test")))
        h.device.signedInTo("server-1")
        val saved = SavedServer("server-1", "Basement", listOf("http://gone.test", "http://home.test"))

        val session = h.core.restore(saved)

        assertTrue(session.isSignedIn)
        assertFalse(session.unreachable.value)
        assertEquals("http://home.test", session.baseUrl)
        assertEquals(listOf(SavedServer("server-1", "Basement", listOf("http://home.test"))), h.core.servers.first())
    }

    @Test
    fun aRenamedServerIsShownAndSavedUnderItsNewName() = runTest {
        val h = Harness(this)
        h.servers.server("home.test", info("server-1", "Kivo"))
        h.device.signedInTo("server-1")
        val session = h.core.restore(SavedServer("server-1", "Kivo", listOf("http://home.test")))
        assertEquals("Kivo", session.savedServer.value.name)

        // The owner renames it while the app is open.
        h.servers.server("home.test", info("server-1", "Den"))
        session.serverInfo()

        assertEquals(SavedServer("server-1", "Den", listOf("http://home.test")), session.savedServer.value)
        assertEquals("Den", session.savedServer.value.name)
        assertEquals(listOf(SavedServer("server-1", "Den", listOf("http://home.test"))), h.core.servers.first())
    }

    @Test
    fun aServerRenamedWhileTheAppWasClosedOpensUnderItsNewName() = runTest {
        val h = Harness(this)
        h.servers.server("home.test", info("server-1", "Den"))
        h.device.signedInTo("server-1")

        val session = h.core.restore(SavedServer("server-1", "Kivo", listOf("http://home.test")))

        assertEquals("Den", session.savedServer.value.name)
        assertEquals("Den", session.savedServer.value.name)
        assertEquals(listOf(SavedServer("server-1", "Den", listOf("http://home.test"))), h.core.servers.first())
    }

    @Test
    fun aRenamedServerThatDoesntAnswerAsTheAppOpensShowsItsNewNameOnceItDoes() = runTest {
        val h = Harness(this)
        // The owner renamed it while the app was closed; as the app opens, no address answers its probe.
        h.servers.on("home.test", "GET", "/api/v1/server") { throw IOException("no route to host") }
        h.device.signedInTo("server-1")
        val session = h.core.restore(SavedServer("server-1", "Kivo", listOf("http://home.test")))
        assertTrue(session.unreachable.value)
        assertEquals("Kivo", session.savedServer.value.name)

        // Then it answers, as Home reads it.
        h.servers.server("home.test", info("server-1", "Den"))
        session.awaitReachable()

        val renamed = withContext(Dispatchers.Default) { withTimeoutOrNull(5.seconds) { session.savedServer.first { it.name != "Kivo" } } }
        assertEquals("Den", renamed?.name)
        assertEquals(listOf(SavedServer("server-1", "Den", listOf("http://home.test"))), h.core.servers.first())
    }

    @Test
    fun comingToTheForegroundReadsTheServersNameAgain() = runTest {
        val h = Harness(this)
        h.servers.server("home.test", info("server-1", "Kivo"))
        h.device.signedInTo("server-1")
        val session = h.core.restore(SavedServer("server-1", "Kivo", listOf("http://home.test")))
        h.servers.server("home.test", info("server-1", "Den"))

        val foreground = backgroundScope.launch { session.runLiveEvents() }

        assertEquals("Den", session.savedServer.first { it.name != "Kivo" }.name)
        assertEquals("Den", h.core.servers.first { all -> all.single().name != "Kivo" }.single().name)
        foreground.cancel()
    }

    @Test
    fun aSignedOutSessionLeavesTheSavedServerAsItIs() = runTest {
        val h = Harness(this)
        h.servers.server("home.test", info("server-1", "Den"))
        val session = h.core.restore(SavedServer("server-1", "Kivo", listOf("http://home.test")))

        session.serverInfo()

        assertEquals("Kivo", session.savedServer.value.name)
        assertEquals(emptyList(), h.core.servers.first())
    }

    @Test
    fun aSavedServerThatDoesntAnswerOpensUnreachableAtItsFirstAddress() = runTest {
        val h = Harness(this)
        h.device.signedInTo("server-1")
        val saved = SavedServer("server-1", "Basement", listOf("http://gone.test", "http://away.test"))

        val session = h.core.restore(saved)

        assertTrue(session.isSignedIn)
        assertTrue(session.unreachable.value)
        assertEquals("http://gone.test", session.baseUrl)
        assertEquals(emptyList(), h.core.servers.first(), "a server that didn't answer isn't saved again")
    }

    @Test
    fun aNetworkChangePicksTheServersAddressAgainAndRequestsFollow() = runTest {
        val h = Harness(this)
        val home = info("server-1", "Basement", localUrls = listOf("http://192.168.1.20:8484")).copy(publicUrl = "https://kivo.example.com")
        h.servers.server("192.168.1.20", home)
        h.device.signedInTo("server-1")
        val session = h.core.restore(SavedServer("server-1", "Basement", listOf("http://192.168.1.20:8484")))
        assertEquals("http://192.168.1.20:8484", session.baseUrl)
        assertEquals(listOf("http://192.168.1.20:8484", "https://kivo.example.com"), h.core.servers.first().single().addresses)

        // The phone leaves home: only the public address answers.
        h.servers.on("192.168.1.20", "GET", "/api/v1/server") { throw IOException("no route to host") }
        h.servers.server("kivo.example.com", home)
        testScheduler.runCurrent()
        h.core.networkChanged(online = true, expensive = true)

        assertEquals("https://kivo.example.com", session.address.first { it != "http://192.168.1.20:8484" })
        assertEquals("https://kivo.example.com/api/v1/images/i1?width=480", session.imageUrl("i1", 480))
        session.account.currentUser()
        assertTrue("kivo.example.com GET /api/v1/users/me" in h.servers.seen)
    }

    @Test
    fun aSavedServerOpensAtALocalAddressThatAnswersBeforeAPublicOne() = runTest {
        val h = Harness(this)
        val home = info("server-1", "Basement", localUrls = listOf("http://192.168.1.20:8484")).copy(publicUrl = "https://kivo.example.com")
        h.servers.server("kivo.example.com", home)
        h.servers.server("192.168.1.20", home)
        h.device.signedInTo("server-1")

        val session = h.core.restore(SavedServer("server-1", "Basement", listOf("https://kivo.example.com", "http://192.168.1.20:8484")))

        assertEquals("http://192.168.1.20:8484", session.baseUrl)
    }

    @Test
    fun restoringWithoutASessionIsSignedOut() = runTest {
        val h = Harness(this)
        h.servers.server("home.test", info("server-1"))

        val session = h.core.restore(SavedServer("server-1", "Basement", listOf("http://home.test")))

        assertFalse(session.isSignedIn)
        assertNull(session.downloads, "a device without downloads has none")
    }

    @Test
    fun theServerToOpenIsTheLastSignedInToThatStillHasASession() = runTest {
        val h = Harness(this)
        h.servers.server("a.test", info("a"))
        h.servers.server("b.test", info("b"))
        h.device.signedInTo("a")
        h.device.signedInTo("b")
        h.core.restore(SavedServer("a", "a", listOf("http://a.test")))
        h.core.restore(SavedServer("b", "b", listOf("http://b.test")))
        assertEquals("b", h.core.lastSignedIn()?.id)

        h.device.secureStore.values.remove("kivo.token.b")

        assertEquals("a", h.core.lastSignedIn()?.id)
    }

    @Test
    fun signingInWithAPasswordSavesTheServerAndOpensItsSession() = runTest {
        val h = Harness(this)
        h.servers.server("home.test", info("server-1", "Basement", localUrls = listOf("http://192.168.1.2:8484")))
        val at = Instant.fromEpochMilliseconds(0)
        h.servers.on("home.test", "POST", "/api/v1/sessions") {
            json(Session.serializer(), Session("s1", "app", "Living Room", at, at, current = true, token = "tok"), HttpStatusCode.Created)
        }

        val session = h.core.signIn(FoundServer("http://home.test", info("server-1", "Basement")), "anna", "secret", "Living Room")

        assertTrue(session.isSignedIn)
        assertEquals("anna", session.user.value?.username)
        assertEquals(listOf("http://192.168.1.2:8484", "http://home.test"), h.core.servers.first().single().addresses)
        assertEquals("server-1", h.core.lastSignedIn()?.id)
        val signIn = h.servers.clients[h.servers.seen.indexOf("home.test POST /api/v1/sessions")]
        assertEquals("kivo-android 1.0", signIn, "the app says who it is as it signs in")
        assertTrue(h.servers.clients.all { it == signIn }, "on every request")
    }

    @Test
    fun signingOutEndsTheSessionOnTheServerAndHere() = runTest {
        val h = Harness(this, withDownloads = true)
        h.servers.server("home.test", info("server-1"))
        h.servers.on("home.test", "DELETE", "/api/v1/sessions/current") { empty() }
        h.device.signedInTo("server-1")
        val session = h.core.restore(SavedServer("server-1", "server-1", listOf("http://home.test")))
        assertNotNull(session.downloads)

        session.signOut()
        session.awaitSignOut()

        assertFalse(session.isSignedIn)
        assertNull(session.user.value)
        assertNull(h.core.lastSignedIn())
        assertTrue("home.test DELETE /api/v1/sessions/current" in h.servers.seen)
        assertEquals(emptyList(), session.downloads!!.items.first())
    }

    @Test
    fun aSessionClosedAsItSignsOutStillDeletesItsDownloadsAndOneLeftSignedInKeepsThem() = runTest {
        val h = Harness(this, withDownloads = true)
        h.servers.server("a.test", info("a"))
        h.servers.server("b.test", info("b"))
        h.device.signedInTo("a")
        h.device.signedInTo("b")
        val store = DownloadDatabase(h.device.downloads!!.driver).downloadsQueries
        store.checked("a", 1, 2)
        store.checked("b", 1, 3)
        val a = h.core.restore(SavedServer("a", "a", listOf("http://a.test")))

        a.close()
        val b = h.core.restore(SavedServer("b", "b", listOf("http://b.test")))
        b.connection.signOut()
        b.close()
        testScheduler.runCurrent()

        assertNotNull(store.serverCheck("a").executeAsOneOrNull(), "leaving a server signed in keeps its downloads")
        assertNull(store.serverCheck("b").executeAsOneOrNull())
    }

    @Test
    fun theDownloadsFollowTheServerOpenedLastAndAClosedSessionStops() = runTest {
        val h = Harness(this, withDownloads = true)
        h.servers.server("a.test", info("a"))
        h.servers.server("b.test", info("b"))
        h.servers.on("b.test", "POST", "/api/v1/downloads/estimate") { json(DownloadEstimateList.serializer(), DownloadEstimateList(emptyList())) }
        h.device.signedInTo("a")
        h.device.signedInTo("b")
        val a = h.core.restore(SavedServer("a", "a", listOf("http://a.test")))

        val b = h.core.restore(SavedServer("b", "b", listOf("http://b.test")))
        a.close()
        b.downloads!!.estimates(listOf("w1"), null, null, null)

        assertTrue("b.test POST /api/v1/downloads/estimate" in h.servers.seen)
        assertFalse(a.scope.isActive)
        assertTrue(b.scope.isActive)
        assertTrue(b.isSignedIn, "leaving a server doesn't sign the next out")
    }

    @Test
    fun aPlayStartsWithThisDevicesSettingsOnTheServersLadder() = runTest {
        val h = Harness(this)
        h.servers.server("home.test", info("server-1"))
        h.device.signedInTo("server-1")
        h.core.updateSettings { it.copy(localQuality = Quality.Limit(8_000_000), remoteQuality = Quality.Limit(3_000_000), dialogueBoost = true) }
        h.core.bandwidth.remember(5_000_000)
        val session = h.core.restore(SavedServer("server-1", "server-1", listOf("http://home.test")))
        val player = session.playback.videoPlayer(FakeEngine())

        val request = player.request("w1", "v1", "a1", null, null)

        assertEquals(QualityRequest("limit", maxBitrateBps = 8_000_000), request.localQuality)
        assertEquals(QualityRequest("auto", bandwidthBps = 5_000_000), request.remoteQuality, "a rung the server doesn't have is Auto")
        assertTrue(request.dialogueBoost)
        assertEquals("v1", request.versionId)
        assertEquals("a1", request.audioTrackId)
        assertNull(request.positionMs)
        assertEquals(0, player.request("w1", null, null, null, 0).positionMs)
        assertEquals(42_000, player.request("w1", null, null, null, 42_000).positionMs)
        player.dispose()
    }

    @Test
    fun aPlayWithoutTheLadderGoesBySettingsAsTheyAre() = runTest {
        val h = Harness(this)
        h.device.signedInTo("server-1")
        h.core.updateSettings { it.copy(localQuality = Quality.Original, remoteQuality = Quality.Limit(3_000_000)) }
        val session = h.core.restore(SavedServer("server-1", "server-1", listOf("http://home.test")))
        val player = session.playback.videoPlayer(FakeEngine())

        val request = player.request("w1", null, null, null, null)

        assertEquals(QualityRequest("original"), request.localQuality)
        assertEquals(QualityRequest("limit", maxBitrateBps = 3_000_000), request.remoteQuality)
        assertFalse(request.dialogueBoost)
        player.dispose()
    }

    @Test
    fun theDialogueBoostSwitchedInThePlayerIsKeptForLaterPlays() = runTest {
        val h = Harness(this)
        h.device.signedInTo("server-1")
        val session = h.core.restore(SavedServer("server-1", "server-1", listOf("http://home.test")))
        val player = session.playback.videoPlayer(FakeEngine())

        player.boostDialogue(true)
        testScheduler.runCurrent()

        assertTrue(h.core.settings.first().dialogueBoost)
        player.dispose()
    }

    @Test
    fun theRunOfEpisodesPlayedOnIsTheAppsAcrossPlayersAndACastCarriesIt() = runTest {
        val h = Harness(this)
        h.servers.server("home.test", info("server-1"))
        h.servers.on("home.test", "POST", "/api/v1/cast-tokens") {
            json(
                CastToken.serializer(),
                CastToken("cast-secret", Instant.parse("2026-10-19T12:00:00Z"), "server-1", listOf("http://home.test")),
                HttpStatusCode.Created,
            )
        }
        h.device.signedInTo("server-1")
        val session = h.core.restore(SavedServer("server-1", "server-1", listOf("http://home.test")))
        val first = session.playback.videoPlayer(FakeEngine())
        repeat(2) { h.core.autoplay.playedOnItsOwn() }
        first.dispose()

        val second = session.playback.videoPlayer(FakeEngine())
        assertEquals(2, h.core.autoplay.count, "a new player keeps the run")
        val load = session.cast.load("Living Room", "w1", null, null, null, null)
        assertTrue("\"autoplayCount\":2" in load.customData)

        second.userActivity()
        assertEquals(0, h.core.autoplay.count, "input in a player starts it again")
        h.core.autoplay.playedOnItsOwn()
        session.playback.confirmStillWatching()
        assertEquals(0, h.core.autoplay.count, "and so does input elsewhere")
        second.dispose()
    }

    @Test
    fun upNextFollowsTheUsersAutoplayReadAgainForEachEpisodeWithANextOne() = runTest {
        val h = Harness(this)
        var autoplayNext = false
        h.servers.server("home.test", info("server-1"))
        h.servers.on("home.test", "GET", "/api/v1/users/me") {
            json(CurrentUser.serializer(), anna.copy(playback = anna.playback.copy(autoplayNext = autoplayNext)))
        }
        h.device.signedInTo("server-1")
        val session = h.core.restore(SavedServer("server-1", "server-1", listOf("http://home.test")))
        val first = session.playback.videoPlayer(FakeEngine())
        first.setHasNextEpisode(hasNext = true)
        assertEquals(UpNextPhase.Waiting, h.core.autoplay.await { it.phase == UpNextPhase.Waiting }?.phase, "off on another device")
        first.dispose()

        autoplayNext = true
        val second = session.playback.videoPlayer(FakeEngine())
        second.setHasNextEpisode(hasNext = true)
        assertEquals(UpNextPhase.CountingDown, h.core.autoplay.await { it.phase == UpNextPhase.CountingDown }?.phase, "on again")
        second.dispose()
    }

    @Test
    fun aCardReadInAListIsKnownAndAWorkReadEarlierShowsAtOnce() = runTest {
        val h = Harness(this)
        h.servers.server("home.test", info("server-1"))
        val summary = WorkSummary("w1", "movie", "Heat", "matched", true, WorkImages(), year = 1995, libraryId = "l1")
        val work = WorkDetail(
            "w1", "movie", "Heat", "matched", true, WorkImages(), emptyList(), emptyList(), emptyList(), emptyList(), emptyList(),
            emptyList(), emptyList(), year = 1995, overview = "A heist.", libraryId = "l1",
        )
        h.servers.on("home.test", "GET", "/api/v1/libraries/l1/works") { json(WorkSummaryPage.serializer(), WorkSummaryPage(listOf(summary), 1)) }
        h.servers.on("home.test", "GET", "/api/v1/works/w1") { json(WorkDetail.serializer(), work) }
        h.device.signedInTo("server-1")
        val session = h.core.restore(SavedServer("server-1", "server-1", listOf("http://home.test")))
        assertNull(session.library.known("w1"))

        session.library.gridPage(GridQuery("l1"), 0, 50)
        assertEquals(summary, session.library.known("w1"))
        assertEquals(Load.Loaded(work), session.library.work("w1").first())

        h.servers.on("home.test", "GET", "/api/v1/works/w1") { throw IOException("offline") }
        assertEquals(Load.Loaded(work), session.library.work("w1").first(), "shown before the server answers")
        assertEquals("Heat", session.library.known("w1")?.title)
    }

    @Test
    fun theLibrariesLastReadShowAtOnceInTheNextSession() = runTest {
        val h = Harness(this)
        h.servers.server("home.test", info("server-1"))
        val libraries = listOf(Library("l1", "Movies", true, "movies", 3))
        h.servers.on("home.test", "GET", "/api/v1/libraries") { json(LibraryList.serializer(), LibraryList(libraries)) }
        h.device.signedInTo("server-1")
        val saved = SavedServer("server-1", "server-1", listOf("http://home.test"))
        val first = h.core.restore(saved)
        assertEquals(Load.Loaded(libraries), first.library.libraries().first(), "none kept before the user is known")
        first.account.currentUser()
        first.library.libraries().first()
        first.close()

        h.servers.on("home.test", "GET", "/api/v1/libraries") { throw IOException("offline") }
        val next = h.core.restore(saved)
        assertEquals(Load.Loaded(libraries), next.library.libraries().first())
    }
}

/** This, once [done] holds of it while the test's tasks run; null after 5 seconds. */
private suspend fun Autoplay.await(done: (Autoplay) -> Boolean): Autoplay? = withContext(Dispatchers.Default) {
    withTimeoutOrNull(5.seconds) {
        while (!done(this@await)) delay(10)
        this@await
    }
}
