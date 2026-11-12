package com.mmcdole.kivo.core.play

import com.mmcdole.kivo.api.apis.PlaysApi
import com.mmcdole.kivo.api.check
import com.mmcdole.kivo.api.models.PlayFallback
import com.mmcdole.kivo.api.models.PlayUpdate
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayCreate
import com.mmcdole.kivo.api.models.PlayStop
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.Reachability

/**
 * Where the player's plays come from: the server's, or a download's own,
 * played in the app without the server. A failure is the API's: a
 * `ProblemException`, a `404` for a play the server has forgotten.
 */
internal interface PlaySource {
    suspend fun start(request: PlayCreate): PlayStarted

    suspend fun heartbeat(id: String, heartbeat: PlayUpdate)

    /**
     * Reports [heartbeat] once the server answers: the play's media got no
     * answer. While none comes, it asks again after a wait, at the server's
     * address picked again each time.
     */
    suspend fun reconnect(id: String, heartbeat: PlayUpdate)

    /** Saves the position and ends the play. */
    suspend fun stop(id: String, stop: PlayStop)

    /** Ends a failed play and starts it again as a transcode. */
    suspend fun fallback(id: String, request: PlayFallback): PlayStarted
}

/** The server's plays. */
internal class ServerPlays(private val api: PlaysApi, private val reachability: Reachability) : PlaySource {
    override suspend fun start(request: PlayCreate): PlayStarted = api.startPlay(request).result()

    override suspend fun heartbeat(id: String, heartbeat: PlayUpdate) = api.updatePlay(id, heartbeat).check()

    override suspend fun reconnect(id: String, heartbeat: PlayUpdate) = reachability.untilAnswered { heartbeat(id, heartbeat) }

    override suspend fun stop(id: String, stop: PlayStop) = api.stopPlay(id, stop).check()

    override suspend fun fallback(id: String, request: PlayFallback): PlayStarted = api.fallbackPlay(id, request).result()
}
