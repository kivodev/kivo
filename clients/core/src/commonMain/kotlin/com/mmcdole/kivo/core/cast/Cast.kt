package com.mmcdole.kivo.core.cast

import com.mmcdole.kivo.api.models.CastToken
import com.mmcdole.kivo.api.models.CastTokenCreate
import com.mmcdole.kivo.api.models.DialogueBoost
import com.mmcdole.kivo.api.models.PlayTrack
import com.mmcdole.kivo.api.models.QualityRequest
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.kivoJson
import com.mmcdole.kivo.core.player.PlayTracks
import com.mmcdole.kivo.core.player.SUBTITLES_OFF
import com.mmcdole.kivo.core.player.TrackShow
import com.mmcdole.kivo.core.player.UpNext
import com.mmcdole.kivo.core.player.UpNextPhase
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonObjectBuilder
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put

// Casting to Kivo's Web Receiver. A sender asks the server for a cast token
// and sends a LOAD naming the work, the server and the token; the receiver
// starts and heartbeats the play itself, as the token's user. The sender
// reads the receiver's status and sends Kivo's own messages on
// CAST_NAMESPACE, never a token anywhere but the LOAD and `kivo.token`.

/** The Google Cast app senders launch: Kivo's own Web Receiver. A fork registers its own and changes it here. */
const val CAST_APP_ID = "2457CC30"

/** The namespace of Kivo's own messages between senders and the receiver. */
const val CAST_NAMESPACE = "urn:x-cast:com.mmcdole.kivo"

/** The version of the contract with the receiver and the server: the LOAD's custom data, the status and the messages. */
internal const val CAST_PROTOCOL = 3

/** What a cast plays: the work, its version and tracks, where it starts, and the choices it carries to the TV. */
internal data class CastRequest(
    val workId: String,
    /** Null for the server's defaults. */
    val versionId: String? = null,
    val audioTrackId: String? = null,
    /** A track's ID, [SUBTITLES_OFF], or null for the server's default. */
    val subtitleTrackId: String? = null,
    /** Where to start; null for the server's resume position. */
    val positionMs: Long? = null,
    val dialogueBoost: Boolean = false,
    val localQuality: QualityRequest = QualityRequest("auto"),
    val remoteQuality: QualityRequest = QualityRequest("auto"),
    /** Episodes played on without input before the cast, so "Still watching?" counts across it. */
    val autoplayCount: Int = 0,
)

/** A LOAD for the receiver: the work as its content ID and entity, the start in seconds, and the custom data as JSON. */
data class CastLoad(
    val contentId: String,
    val entity: String,
    /** Null for the server's resume position. */
    val currentTimeSec: Double?,
    val customData: String,
)

/** What goes to the receiver leaves out what isn't set, as the receiver passes it on to the server. */
private val wireJson = Json { explicitNulls = false }

/** The LOAD for [request], carrying [token] and its server's ID and addresses. */
internal fun castLoad(token: CastToken, request: CastRequest): CastLoad {
    val customData = buildJsonObject {
        put("protocol", CAST_PROTOCOL)
        put("serverId", token.serverId)
        put("urls", wireJson.encodeToJsonElement(token.urls))
        put("token", token.token)
        request.versionId?.let { put("versionId", it) }
        request.audioTrackId?.let { put("audioTrackId", it) }
        request.subtitleTrackId?.let { put("subtitleTrackId", it) }
        put("dialogueBoost", request.dialogueBoost)
        put("localQuality", wireJson.encodeToJsonElement(request.localQuality))
        put("remoteQuality", wireJson.encodeToJsonElement(request.remoteQuality))
        put("autoplayCount", request.autoplayCount)
    }
    return CastLoad(
        contentId = request.workId,
        entity = "kivo://work/${request.workId}",
        currentTimeSec = request.positionMs?.let { it / 1000.0 },
        customData = customData.toString(),
    )
}

/**
 * The server side of casting for a signed-in app: cast tokens for a
 * receiver, made fresh for each LOAD and for a sender that reconnects.
 */
internal class CastSender(private val connection: ServerConnection) {
    /** A cast token for the Web Receiver on the Cast device [receiverName], such as "Living Room TV". */
    suspend fun token(receiverName: String): CastToken =
        connection.sessions.createCastToken(CastTokenCreate(receiverName.take(RECEIVER_NAME_MAX), RECEIVER_WEB)).result()

    /** The LOAD for [request] on [receiverName], with a new cast token. */
    suspend fun load(receiverName: String, request: CastRequest): CastLoad = castLoad(token(receiverName), request)

    /** `kivo.token` with a new cast token, which a sender that reconnects sends so the receiver plays on. */
    suspend fun tokenMessage(receiverName: String): String = CastMessages.token(token(receiverName).token)

    private companion object {
        const val RECEIVER_WEB = "web"
        const val RECEIVER_NAME_MAX = 100
    }
}

/** The messages senders send the receiver on [CAST_NAMESPACE], as JSON. */
internal object CastMessages {
    /** Another audio track, a subtitle track or [SUBTITLES_OFF], or both; the receiver starts a new play where it must. */
    fun setTracks(audioTrackId: String?, subtitleTrackId: String?): String = message("kivo.setTracks") {
        audioTrackId?.let { put("audioTrackId", it) }
        subtitleTrackId?.let { put("subtitleTrackId", it) }
    }

    fun setDialogueBoost(enabled: Boolean): String = message("kivo.setDialogueBoost") { put("enabled", enabled) }

    fun setQuality(localQuality: QualityRequest, remoteQuality: QualityRequest): String = message("kivo.setQuality") {
        put("localQuality", wireJson.encodeToJsonElement(localQuality))
        put("remoteQuality", wireJson.encodeToJsonElement(remoteQuality))
    }

    /** Another version, with its own default tracks. */
    fun setVersion(versionId: String): String = message("kivo.setVersion") { put("versionId", versionId) }

    /** Up next's Play now: the next episode plays at once. */
    fun playNext(): String = message("kivo.playNext") {}

    /** Up next's Hide: no card for the rest of the episode. */
    fun hideUpNext(): String = message("kivo.hideUpNext") {}

    /** A fresh cast token for the receiver's play. */
    fun token(token: String): String = message("kivo.token") { put("token", token) }

    /** Asks the receiver for `kivo.status`. */
    fun getStatus(): String = message("kivo.getStatus") {}

    private fun message(type: String, fields: JsonObjectBuilder.() -> Unit): String =
        buildJsonObject {
            put("type", type)
            fields()
        }.toString()
}

/**
 * What the receiver says of its cast, from its `kivo.status` message or a
 * media status' custom data. Senders draw their menus, stream info, up
 * next and errors from it.
 */
data class CastStatus(
    val workId: String?,
    val nextWorkId: String?,
    val versionId: String?,
    /** The receiver's play's tracks, as a play gives them: how each shows, and its labels. */
    val tracks: List<PlayTrack>,
    val quality: QualityRequest?,
    val limitBps: Long?,
    val dialogueBoost: DialogueBoost?,
    val audioTrackId: String?,
    /** The subtitle showing, or [SUBTITLES_OFF]; null with no play. */
    val subtitleTrackId: String?,
    /** Whether the receiver plays from the server's local network; null with no play. */
    val local: Boolean?,
    /** Up next's card on the TV, mirrored: Play now and Hide act on it. */
    val upNext: UpNext,
    /** Why the cast stopped or can't go on, such as `serverUnreachable` or `signedOut`; null while it's well. */
    val error: String?,
    /** An admin's words when they stopped the play. */
    val message: String?,
) {
    /** Whether the receiver has a play. */
    val playing: Boolean get() = versionId != null

    /**
     * Whether choosing [trackId], a track's ID or [SUBTITLES_OFF], takes
     * the receiver a new play at the position, by its play's tracks; else it
     * shows the choice at once.
     */
    fun startsNewPlay(trackId: String): Boolean = PlayTracks.show(tracks, trackId) == TrackShow.NewPlay

    /** Whether the receiver can't reach the server, which it keeps trying. */
    val unreachable: Boolean get() = error == ERROR_UNREACHABLE

    companion object {
        const val ERROR_UNREACHABLE = "serverUnreachable"

        /**
         * The receiver's status in [json]: a `kivo.status` message, a media
         * status' custom data (`{"status": {…}}`), or the status itself; null
         * when it holds none.
         */
        internal fun parseOrNull(json: String): CastStatus? {
            val element = runCatching { kivoJson.parseToJsonElement(json) }.getOrNull() as? JsonObject ?: return null
            val status: JsonElement = (element["status"] as? JsonObject) ?: element
            val wire = runCatching { kivoJson.decodeFromJsonElement(WireStatus.serializer(), status.jsonObject) }.getOrNull() ?: return null
            return CastStatus(
                workId = wire.workId,
                nextWorkId = wire.nextWorkId,
                versionId = wire.versionId,
                tracks = wire.tracks.orEmpty(),
                quality = wire.quality,
                limitBps = wire.limitBps,
                dialogueBoost = wire.dialogueBoost,
                audioTrackId = wire.audioTrackId,
                subtitleTrackId = wire.subtitleTrackId ?: SUBTITLES_OFF.takeIf { wire.versionId != null },
                local = wire.local,
                upNext = wire.upNext?.let { UpNext(phaseOf(it.phase), it.secondsLeft) } ?: UpNext(),
                error = wire.error,
                message = wire.message,
            )
        }
    }
}

private fun phaseOf(phase: String): UpNextPhase = when (phase) {
    "countingDown" -> UpNextPhase.CountingDown
    "stillWatching" -> UpNextPhase.StillWatching
    "dismissed" -> UpNextPhase.Dismissed
    else -> UpNextPhase.Hidden
}

@Serializable
private class WireStatus(
    val workId: String? = null,
    val nextWorkId: String? = null,
    val versionId: String? = null,
    val tracks: List<PlayTrack>? = null,
    val quality: QualityRequest? = null,
    val limitBps: Long? = null,
    val dialogueBoost: DialogueBoost? = null,
    val audioTrackId: String? = null,
    val subtitleTrackId: String? = null,
    val local: Boolean? = null,
    val upNext: WireUpNext? = null,
    val error: String? = null,
    val message: String? = null,
)

@Serializable
private class WireUpNext(val phase: String, val secondsLeft: Int = 0)
