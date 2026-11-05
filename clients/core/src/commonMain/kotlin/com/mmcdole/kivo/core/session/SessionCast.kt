package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.core.cast.CastLoad
import com.mmcdole.kivo.core.cast.CastRequest
import com.mmcdole.kivo.core.cast.CastSender

/** Casting to a Cast device ([KivoSession.cast]): what a cast's LOAD carries, and a cast token for one joined again. */
class SessionCast internal constructor(private val session: KivoSession) {
    private val sender = CastSender(session.connection)

    /**
     * The LOAD that casts [workId] to the Cast device [receiverName], with a
     * new cast token, this device's local and remote qualities and dialogue
     * boost, and the run of episodes this app's players played on without
     * input. A null [positionMs] starts at the resume position. A video cast
     * stops the book playing here, keeping its place.
     */
    @Throws(Exception::class)
    suspend fun load(
        receiverName: String,
        workId: String,
        versionId: String?,
        audioTrackId: String?,
        subtitleTrackId: String?,
        positionMs: Long?,
    ): CastLoad {
        session.playback.stopBook()
        val settings = session.playback.playSettings()
        return sender.load(
            receiverName,
            CastRequest(
                workId = workId,
                versionId = versionId,
                audioTrackId = audioTrackId,
                subtitleTrackId = subtitleTrackId,
                positionMs = positionMs,
                dialogueBoost = settings.dialogueBoost,
                localQuality = settings.localQuality.request(),
                remoteQuality = settings.remoteQuality.request(),
                autoplayCount = session.core.autoplay.count,
            ),
        )
    }

    /** `kivo.token` with a new cast token for [receiverName], for a cast this app joins again. */
    @Throws(Exception::class)
    suspend fun tokenMessage(receiverName: String): String = sender.tokenMessage(receiverName)
}
