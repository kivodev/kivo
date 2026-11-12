package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.core.cast.CastLink
import com.mmcdole.kivo.core.cast.CastLoad
import com.mmcdole.kivo.core.cast.CastRequest
import com.mmcdole.kivo.core.cast.CastSender
import com.mmcdole.kivo.core.play.PlayChoice

/** Casting to a Cast device ([KivoSession.cast]). */
class SessionCast internal constructor(private val session: KivoSession) {
    private val sender = CastSender(session.connection)

    /** The remote for the Cast session [link] reports, which it follows from now until it's closed. */
    fun remote(link: CastLink): CastRemote = CastRemote(session, link)

    /**
     * The LOAD that casts [choice] to the Cast device [receiverName], with a
     * new cast token, this device's local and remote qualities and dialogue
     * boost, and the run of episodes this app's players played on without
     * input. A null [positionMs] starts at the resume position. A video cast
     * stops the book playing here, keeping its place.
     */
    internal suspend fun load(receiverName: String, choice: PlayChoice, positionMs: Long? = null): CastLoad {
        session.playback.stopBook()
        val settings = session.playback.playSettings()
        return sender.load(
            receiverName,
            CastRequest(
                workId = choice.workId,
                versionId = choice.versionId,
                audioTrackId = choice.audioTrackId,
                subtitleTrackId = choice.subtitleTrackId,
                positionMs = positionMs,
                dialogueBoost = settings.dialogueBoost,
                localQuality = settings.localQuality.request(),
                remoteQuality = settings.remoteQuality.request(),
                autoplayCount = session.core.autoplay.count,
            ),
        )
    }

    /** `kivo.token` with a new cast token for [receiverName], for a cast this app joins again. */
    internal suspend fun tokenMessage(receiverName: String): String = sender.tokenMessage(receiverName)
}
