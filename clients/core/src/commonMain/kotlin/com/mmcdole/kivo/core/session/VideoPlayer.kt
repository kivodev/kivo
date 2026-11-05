package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.core.downloads.DownloadSource
import com.mmcdole.kivo.core.downloads.OfflinePlay
import com.mmcdole.kivo.core.play.PlayController
import com.mmcdole.kivo.core.play.PlayRequest
import com.mmcdole.kivo.core.play.PlaySource
import com.mmcdole.kivo.core.play.PlayerUi
import com.mmcdole.kivo.core.play.PlayerUiRules
import com.mmcdole.kivo.core.player.PlayerEngine
import com.mmcdole.kivo.core.settings.Quality
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

/**
 * The video player ([SessionPlayback.videoPlayer]): the core's [PlayController]
 * over the app's [engine], started with this device's capability report,
 * qualities and dialogue boost, and stopped with a message when the live
 * events say its play ended elsewhere; and what shows over the video ([ui]),
 * by the timed rules every player keeps, from the app's inputs. The app
 * follows [controller]'s state and drives its transport (pause, seek, retry
 * and the like) directly; everything that starts a play goes through here.
 * Its plays are the server's, or with [offline] a download's on the device,
 * which has nothing to fall back to and no download rate to measure.
 */
class VideoPlayer internal constructor(
    private val session: KivoSession,
    private val engine: PlayerEngine,
    plays: PlaySource,
    private val offline: OfflinePlay?,
) {
    private val core = session.core
    private val online = offline == null

    val controller: PlayController = PlayController(
        plays = plays,
        engine = engine,
        capabilities = { external -> core.report.report(external) },
        scope = core.scope,
        canFallBack = online,
        onEnded = { rules.ended() },
    )

    private val rules: PlayerUiRules = PlayerUiRules(controller.state, controller::seek, core.autoplay, session.scope, core.clock)

    /**
     * The controls, Skip Intro, Recap or Credits, up next and where the player
     * goes once it's done ([PlayerUi.end]), which the app carries out.
     */
    val ui: StateFlow<PlayerUi> = rules.ui

    private val ended = session.scope.launch {
        session.playsEnded.collect { controller.playEnded(it.playId, it.message) }
    }

    /** A stream playing holds the server's address where it is while it answers. */
    private val streaming = session.scope.launch {
        if (online) {
            controller.state.map { it.live && !it.paused }.distinctUntilChanged()
                .collect { session.connection.address.streaming(this@VideoPlayer, it) }
        }
    }

    /**
     * This video is an episode, with a next one to play or not, once the app
     * knows: up next and the end follow it, and the user's autoplay, read
     * again from the server when there's a next one.
     */
    fun setHasNextEpisode(hasNext: Boolean) {
        if (online && hasNext) session.account.readAutoplay()
        rules.episode(hasNext)
    }

    /** Any input: someone is watching, so up next may play on again. */
    fun userActivity() = rules.input()

    /** Input that shows the controls, such as a tap, a mouse move or a key. */
    fun showControls() = rules.showControls()

    /** The controls hide: a while after the last input while playing, with no menu open or scrub held, or as Back hides them. */
    fun hideControls() = rules.hideControls()

    /** A tap on the video: the controls hide while playing with no menu open, and show otherwise. */
    fun toggleControls() = rules.toggleControls()

    /** A menu or panel opened over the player, showing the controls and holding up next's countdown; or it closed. */
    fun setMenuOpen(open: Boolean) = rules.menu(open)

    /** A scrub is held, showing the controls; or it let go. */
    fun setScrubbing(held: Boolean) = rules.scrubbing(held)

    /** Skip Intro, Recap or Credits: to the marker's end, with no button for that marker after. */
    fun skipMarker() = rules.skipMarker()

    /** Up next's Hide or Watch credits: no card for the rest of the episode, which still plays on at its end. */
    fun hideUpNext() = rules.hideUpNext()

    /** Up next's Play now, or Next: [PlayerUi.end] becomes the next episode. */
    fun playNext() = rules.playNext()

    /**
     * Starts [workId] with this device's local and remote qualities and its
     * dialogue boost; the server picks between the qualities, Auto by the
     * download rate last measured, or measured first. Null tracks and
     * version are the server's defaults. A null [positionMs] starts at the
     * resume position, 0 at the start.
     */
    @Throws(Exception::class)
    suspend fun play(workId: String, versionId: String?, audioTrackId: String?, subtitleTrackId: String?, positionMs: Long?) {
        controller.play(request(workId, versionId, audioTrackId, subtitleTrackId, positionMs))
    }

    /** Where the file playing came from, for the stream info's "Source" line; null for the server's play. */
    fun downloadSource(): DownloadSource? = offline?.source()

    /** The play [play] starts. */
    internal suspend fun request(
        workId: String,
        versionId: String?,
        audioTrackId: String?,
        subtitleTrackId: String?,
        positionMs: Long?,
    ): PlayRequest {
        val settings = if (online) session.playback.playSettings() else core.settingsNow()
        val auto = settings.localQuality == Quality.Auto || settings.remoteQuality == Quality.Auto
        val bandwidthBps = if (online && auto) core.bandwidth.forAuto(session.connection) else null
        return PlayRequest(
            workId = workId,
            versionId = versionId,
            audioTrackId = audioTrackId,
            subtitleTrackId = subtitleTrackId,
            localQuality = settings.localQuality.request(bandwidthBps),
            remoteQuality = settings.remoteQuality.request(bandwidthBps),
            positionMs = positionMs,
            dialogueBoost = settings.dialogueBoost,
        )
    }

    /** Switches the dialogue boost, kept on this device for later plays, and plays on from the position with it. */
    fun boostDialogue(on: Boolean) {
        core.scope.launch { core.setDialogueBoost(on) }
        controller.boostDialogue(on)
    }

    /** Plays an audio track: at once from the file, or by a new play at the position ([PlayController.chooseAudio]). */
    fun chooseAudio(trackId: String) = controller.chooseAudio(trackId)

    /** Shows a subtitle track, or `off`: at once, or by a new play at the position ([PlayController.chooseSubtitle]). */
    fun chooseSubtitle(trackId: String) = controller.chooseSubtitle(trackId)

    /**
     * Whether the app can draw over the video: not in picture-in-picture
     * or on an AirPlay device ([PlayController.setOverlayVisible]). A download
     * has no transcode to burn one in, so it draws as it can.
     */
    fun setOverlayVisible(visible: Boolean) {
        if (online) controller.setOverlayVisible(visible)
    }

    /** A quality for this video, at home or away, keeping the subtitle showing. */
    fun chooseQuality(quality: Quality) = controller.switchTo {
        it.copy(localQuality = quality.request(), remoteQuality = quality.request(), subtitleTrackId = controller.state.value.subtitleTrackId)
    }

    /** Another version, with its own default tracks. */
    fun chooseVersion(versionId: String) =
        controller.switchTo { it.copy(versionId = versionId, audioTrackId = null, subtitleTrackId = null) }

    /** Stops the play and releases the engine, keeping the download rate it measured away from home; the player is done. */
    fun dispose() {
        close()
        controller.dispose()
    }

    /**
     * Stops the play as [dispose] does, but leaves its media in the engine,
     * paused, for the next player on the engine, such as the next episode's,
     * to replace with its own: the picture never goes empty between them.
     */
    fun handOver() {
        close()
        controller.handOver()
    }

    private fun close() {
        val measured = engine.bandwidthBps?.takeIf { online && controller.current?.local == false }
        if (measured != null) core.scope.launch { core.bandwidth.remember(measured) }
        ended.cancel()
        streaming.cancel()
        if (online) session.connection.address.streaming(this, false)
        rules.dispose()
    }
}

