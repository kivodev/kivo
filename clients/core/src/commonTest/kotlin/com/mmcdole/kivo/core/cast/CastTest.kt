package com.mmcdole.kivo.core.cast

import com.mmcdole.kivo.api.models.CastToken
import com.mmcdole.kivo.api.models.QualityRequest
import com.mmcdole.kivo.core.FakeServer
import com.mmcdole.kivo.core.MemoryStore
import com.mmcdole.kivo.core.json
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.player.Subtitles
import com.mmcdole.kivo.core.player.UpNext
import com.mmcdole.kivo.core.player.UpNextPhase
import io.ktor.http.HttpStatusCode
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlin.time.Instant

class CastTest {
    private val token = CastToken(
        token = "cast-secret",
        expiresAt = Instant.parse("2026-10-19T12:00:00Z"),
        serverId = "server-1",
        urls = listOf("http://10.0.0.1:8484", "https://kivo.example.com"),
    )

    private fun parse(text: String): JsonObject = Json.parseToJsonElement(text).jsonObject

    @Test
    fun theLoadCarriesTheWorkTheServerTheTokenAndTheChoicesAsTheReceiverReadsThem() {
        val load = castLoad(
            token,
            CastRequest(
                workId = "w1",
                versionId = "v2",
                audioTrackId = "a3",
                subtitleTrackId = Subtitles.OFF,
                positionMs = 2_723_500,
                dialogueBoost = true,
                localQuality = QualityRequest("original"),
                remoteQuality = QualityRequest("limit", maxBitrateBps = 8_000_000),
                autoplayCount = 2,
            ),
        )

        assertEquals("w1", load.contentId)
        assertEquals("kivo://work/w1", load.entity)
        assertEquals(2723.5, load.currentTimeSec)
        assertEquals(
            parse(
                """
                {"protocol":3,"serverId":"server-1","urls":["http://10.0.0.1:8484","https://kivo.example.com"],
                 "token":"cast-secret","versionId":"v2","audioTrackId":"a3","subtitleTrackId":"off",
                 "dialogueBoost":true,"localQuality":{"mode":"original"},
                 "remoteQuality":{"mode":"limit","maxBitrateBps":8000000},"autoplayCount":2}
                """,
            ),
            parse(load.customData),
        )
    }

    @Test
    fun choicesLeftToTheServerAreLeftOutAndNoPositionResumes() {
        val load = castLoad(token, CastRequest(workId = "w1"))

        val custom = parse(load.customData)
        assertNull(load.currentTimeSec)
        assertFalse("versionId" in custom)
        assertFalse("audioTrackId" in custom)
        assertFalse("subtitleTrackId" in custom)
        assertEquals(parse("""{"mode":"auto"}"""), custom["localQuality"])
    }

    @Test
    fun aLoadAsksTheServerForACastTokenForTheReceiverAsThisApp() = runTest {
        val server = FakeServer()
        val connection = server.connection(MemoryStore(mutableMapOf("kivo.token.server-1" to "app-token")))
        connection.restore()
        server.on("POST", "/api/v1/cast-tokens") { json(CastToken.serializer(), token, HttpStatusCode.Created) }

        val load = CastSender(connection).load("Living Room TV", CastRequest(workId = "w1"))

        val sent = server.to("POST", "/api/v1/cast-tokens").single()
        assertEquals("Bearer app-token", sent.authorization)
        assertEquals(parse("""{"receiverName":"Living Room TV","receiverKind":"web"}"""), parse(sent.body))
        assertEquals("cast-secret", parse(load.customData)["token"].toString().trim('"'))
    }

    @Test
    fun aLongDeviceNameIsCutToWhatTheServerTakes() = runTest {
        val server = FakeServer()
        server.on("POST", "/api/v1/cast-tokens") { json(CastToken.serializer(), token, HttpStatusCode.Created) }

        CastSender(server.connection()).token("x".repeat(140))

        val name = parse(server.to("POST", "/api/v1/cast-tokens").single().body)["receiverName"].toString().trim('"')
        assertEquals(100, name.length)
    }

    @Test
    fun aReconnectingSenderSendsAFreshToken() = runTest {
        val server = FakeServer()
        server.on("POST", "/api/v1/cast-tokens") { json(CastToken.serializer(), token, HttpStatusCode.Created) }

        val message = CastSender(server.connection()).tokenMessage("Living Room TV")

        assertEquals(parse("""{"type":"kivo.token","token":"cast-secret"}"""), parse(message))
    }

    @Test
    fun messagesAreTheOnesTheReceiverAnswers() {
        assertEquals(parse("""{"type":"kivo.setTracks","audioTrackId":"a2"}"""), parse(CastMessages.setTracks("a2", null)))
        assertEquals(parse("""{"type":"kivo.setTracks","subtitleTrackId":"off"}"""), parse(CastMessages.setTracks(null, Subtitles.OFF)))
        assertEquals(parse("""{"type":"kivo.setDialogueBoost","enabled":true}"""), parse(CastMessages.setDialogueBoost(true)))
        assertEquals(
            parse("""{"type":"kivo.setQuality","localQuality":{"mode":"limit","maxBitrateBps":4000000},"remoteQuality":{"mode":"limit","maxBitrateBps":4000000}}"""),
            parse(CastMessages.setQuality(QualityRequest("limit", maxBitrateBps = 4_000_000), QualityRequest("limit", maxBitrateBps = 4_000_000))),
        )
        assertEquals(parse("""{"type":"kivo.setVersion","versionId":"v9"}"""), parse(CastMessages.setVersion("v9")))
        assertEquals(parse("""{"type":"kivo.playNext"}"""), parse(CastMessages.playNext()))
        assertEquals(parse("""{"type":"kivo.hideUpNext"}"""), parse(CastMessages.hideUpNext()))
        assertEquals(parse("""{"type":"kivo.getStatus"}"""), parse(CastMessages.getStatus()))
    }

    @Test
    fun theReceiversStatusMessageReadsAsTheCastsState() {
        val status = CastStatus.parseOrNull(
            """
            {"type":"kivo.status","error":null,"workId":"w1","nextWorkId":"w2","versionId":"v1",
             "tracks":[{"trackId":"t0","type":"video","selected":true,"delivery":"stream"},
                       {"trackId":"s1","type":"subtitle","delivery":"stream","codec":"webvtt"},{"trackId":"s2","type":"subtitle"},
                       {"trackId":"off","type":"subtitle","selected":true,"delivery":"none"}],
             "quality":{"mode":"auto"},"dialogueBoost":{"enabled":true,"cost":"audioAsAac"},
             "audioTrackId":"a1","local":true,
             "upNext":{"phase":"countingDown","secondsLeft":7}}
            """,
        )!!

        assertEquals("w1", status.workId)
        assertEquals("w2", status.nextWorkId)
        assertEquals("v1", status.versionId)
        assertEquals("Direct Stream", Display.forLocale("en").method(status.tracks.first()))
        assertEquals(true, status.dialogueBoost?.enabled)
        assertEquals("a1", status.audioTrackId)
        assertEquals(Subtitles.OFF, status.subtitleTrackId)
        assertFalse(status.startsNewPlay("s1"), "streamed: shown at once")
        assertTrue(status.startsNewPlay("s2"))
        assertEquals(true, status.local)
        assertEquals(UpNext(UpNextPhase.CountingDown, 7), status.upNext)
        assertTrue(status.playing)
        assertNull(status.error)
    }

    @Test
    fun aMediaStatusCarriesItUnderStatus() {
        val status = CastStatus.parseOrNull("""{"status":{"error":"serverUnreachable","workId":"w1","versionId":"v1","subtitleTrackId":"s1","upNext":{"phase":"stillWatching","secondsLeft":10}}}""")!!

        assertTrue(status.unreachable)
        assertEquals("s1", status.subtitleTrackId)
        assertEquals(UpNextPhase.StillWatching, status.upNext.phase)
    }

    @Test
    fun theStatusIsReadNeverTheReceiversCopyForEarlierSenders() {
        val custom = CastStatus.parseOrNull("""{"status":{"workId":"w1","upNext":{"phase":"countingDown","secondsLeft":5}},"kivo":{"workId":"w1","upNext":{"phase":"showing","secondsLeft":5}}}""")!!
        assertEquals(UpNextPhase.CountingDown, custom.upNext.phase)
        val message = CastStatus.parseOrNull("""{"type":"kivo.status","workId":"w1","upNext":{"phase":"countingDown","secondsLeft":5},"kivo":{"upNext":{"phase":"showing","secondsLeft":5}}}""")!!
        assertEquals(UpNextPhase.CountingDown, message.upNext.phase)
    }

    @Test
    fun anIdleReceiverHasNoPlayAndNoSubtitle() {
        val status = CastStatus.parseOrNull("""{"type":"kivo.status","error":"signedOut","message":null}""")!!

        assertFalse(status.playing)
        assertNull(status.subtitleTrackId)
        assertEquals("signedOut", status.error)
        assertEquals(UpNext(), status.upNext)
    }

    @Test
    fun anythingElseIsNoStatus() {
        assertNull(CastStatus.parseOrNull("not json"))
        assertNull(CastStatus.parseOrNull("[1,2]"))
        assertNull(CastStatus.parseOrNull("""{"upNext":"soon"}"""))
    }
}
