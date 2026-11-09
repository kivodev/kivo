package com.mmcdole.kivo.android.mobile.settings

import com.mmcdole.kivo.android.mobile.CoreTest
import com.mmcdole.kivo.android.mobile.anna
import com.mmcdole.kivo.android.mobile.awaitValue
import com.mmcdole.kivo.android.mobile.waitFor
import com.mmcdole.kivo.android.mobile.refuse
import com.mmcdole.kivo.android.mobile.reply
import com.mmcdole.kivo.android.settings.ScreenSettingsStore
import com.mmcdole.kivo.android.settings.SubtitleSize
import com.mmcdole.kivo.android.settings.movedRow
import com.mmcdole.kivo.api.models.HomeRowInfo
import com.mmcdole.kivo.api.models.CurrentUser
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryList
import com.mmcdole.kivo.api.models.ServerInfo
import com.mmcdole.kivo.api.models.Session
import com.mmcdole.kivo.api.models.SessionList
import com.mmcdole.kivo.api.models.QualityRung
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.settings.DownloadQuality
import com.mmcdole.kivo.core.settings.Quality
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpStatusCode
import io.ktor.http.content.OutgoingContent
import io.ktor.http.content.TextContent
import java.io.File
import java.util.concurrent.CopyOnWriteArrayList
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue
import kotlin.time.Instant

class SettingsViewModelsTest : CoreTest() {
    private val sent = CopyOnWriteArrayList<String>()
    private var user = anna

    init {
        server.get("/api/v1/server", ServerInfo.serializer()) {
            ServerInfo("s1", "Den", "1.0", qualityLadder = listOf(QualityRung(bitrateBps = 8_000_000, height = 1080), QualityRung(bitrateBps = 3_000_000, height = 720)))
        }
        server.get("/api/v1/users/me", CurrentUser.serializer()) { user }
        server.on("PATCH", "/api/v1/users/me") { request ->
            sent += request.text()
            user = user.copy(playback = user.playback.copy(autoplayNext = !user.playback.autoplayNext))
            reply(CurrentUser.serializer(), user)
        }
        val now = Instant.fromEpochMilliseconds(0)
        server.get("/api/v1/sessions", SessionList.serializer()) {
            SessionList(listOf(Session("x", "app", "Phone", now, now, true), Session("y", "app", "Living Room", now, now, false)))
        }
        server.on("DELETE", "/api/v1/sessions/y") {
            sent += "signOut y"
            respond("", HttpStatusCode.NoContent)
        }
        server.on("POST", "/api/v1/users/me/change-password") { request ->
            if (request.text().contains("\"currentPassword\":\"wrong\"")) refuse(HttpStatusCode.BadRequest, "wrongPassword") else respond("", HttpStatusCode.NoContent)
        }
        server.get("/api/v1/libraries", LibraryList.serializer()) {
            LibraryList(listOf(Library("l1", "Movies", pinned = true, kind = "movies", workCount = 1), Library("l2", "Shows", pinned = false, kind = "shows", workCount = 1)))
        }
    }

    private fun io.ktor.client.request.HttpRequestData.text(): String = when (val content = body) {
        is TextContent -> content.text
        is OutgoingContent.ByteArrayContent -> content.bytes().decodeToString()
        else -> ""
    }

    private val screen by lazy { ScreenSettingsStore(PreferenceDataStoreFactory.create { File(folder, "screen.preferences_pb") }) }

    @Test
    fun preferencesReadTheAccountAndTheLadderAndEachChangeApplies() {
        val model = PreferencesViewModel(signIn(), client, screen)
        val state = awaitValue(model.state) { it.user is Load.Loaded && it.qualities.isNotEmpty() && it.device != null }
        assertEquals(listOf(Quality.Auto, Quality.Original, Quality.Limit(8_000_000), Quality.Limit(3_000_000)), state.qualities.map { it.quality })
        assertEquals(DownloadQuality.Standard, state.device?.downloadQuality, "a phone downloads Standard until it's set")

        model.setAutoplayNext(false)
        assertEquals(false, awaitValue(model.state) { (it.user as? Load.Loaded)?.value?.playback?.autoplayNext == false }.user.getOrNull()?.playback?.autoplayNext)
        assertTrue(sent.single().contains("\"autoplayNext\":false"), sent.single())

        model.setRemoteQuality(Quality.Limit(3_000_000))
        model.setCellularDownloads(true)
        model.setSubtitleSize(SubtitleSize.Large)
        val after = awaitValue(model.state) { it.device?.cellularDownloads == true && it.device?.remoteQuality == Quality.Limit(3_000_000) && it.screen.subtitleSize == SubtitleSize.Large }
        assertEquals(null, after.failure)
    }

    @Test
    fun signedInDevicesListAndSignAnotherOut() {
        var signedOutHere = false
        val model = SessionsViewModel(signIn()) { signedOutHere = true }
        val sessions = assertNotNull(awaitValue(model.state) { it.sessions is Load.Loaded }.sessions.getOrNull())
        assertEquals(listOf("Phone", "Living Room"), sessions.map { it.name })
        model.signOut(sessions[1])
        waitFor { sent.contains("signOut y") }
        model.signOut(sessions[0])
        assertTrue(signedOutHere, "signing out this one signs the app out")
    }

    @Test
    fun aWrongCurrentPasswordSaysSoAndARightOneChangesIt() {
        val model = ChangePasswordViewModel(signIn())
        model.change("wrong", "longenough", signOutOthers = true)
        assertTrue(awaitValue(model.state) { !it.changing && it.wrongPassword }.wrongPassword)
        model.change("right", "short", signOutOthers = true)
        assertEquals(false, model.state.value.changing, "too short a password isn't sent")
        model.change("right", "longenough", signOutOthers = false)
        assertTrue(awaitValue(model.state) { it.changed }.changed)
    }

    @Test
    fun librariesPinAndMoveAndSendTheWholeOrder() {
        val model = LibrariesSettingsViewModel(signIn())
        awaitValue(model.state) { it.libraries.size == 2 }
        model.move("l2", -1)
        model.pin("l2", true)
        waitFor { sent.size == 2 }
        val state = model.state.value
        assertEquals(listOf("l2", "l1"), state.libraries.map { it.id })
        assertTrue(sent.last().contains("""[{"id":"l2","pinned":true},{"id":"l1","pinned":true}]"""), sent.last())
    }

    @Test
    fun movingAHomeRowPassesOverKindsThePhoneDoesntList() {
        val watching = HomeRowInfo("continueWatching", "continueWatching", true)
        val unknown = HomeRowInfo("later", "aKindFromLater", true)
        val recent = HomeRowInfo("recentlyAdded:a", "recentlyAdded", true)
        val all = listOf(watching, unknown, recent)

        assertEquals(listOf(recent, unknown, watching), movedRow(all, all.filter { it.kind != "aKindFromLater" }, "recentlyAdded:a", -1))
        assertEquals(all, movedRow(all, all.filter { it.kind != "aKindFromLater" }, "continueWatching", -1))
    }
}
