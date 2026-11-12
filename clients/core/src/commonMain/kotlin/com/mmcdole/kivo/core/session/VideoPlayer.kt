package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.api.models.PlayCreate
import com.mmcdole.kivo.api.models.PlayFallback
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PlayStop
import com.mmcdole.kivo.api.models.PlayUpdate
import com.mmcdole.kivo.api.models.QualityRung
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.core.answer
import com.mmcdole.kivo.core.data.withTrack
import com.mmcdole.kivo.core.display.qualityMenu
import com.mmcdole.kivo.core.downloads.DownloadStatus
import com.mmcdole.kivo.core.downloads.OfflinePlay
import com.mmcdole.kivo.core.play.PlayChoice
import com.mmcdole.kivo.core.play.PlayController
import com.mmcdole.kivo.core.play.PlayRequest
import com.mmcdole.kivo.core.play.PlaySource
import com.mmcdole.kivo.core.play.PlayerEnd
import com.mmcdole.kivo.core.play.PlayerUi
import com.mmcdole.kivo.core.play.PlayerUiRules
import com.mmcdole.kivo.core.play.ServerPlays
import com.mmcdole.kivo.core.player.PlayerEngine
import com.mmcdole.kivo.core.quietly
import com.mmcdole.kivo.core.settings.Quality
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.distinctUntilChangedBy
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import kotlin.time.Duration

/**
 * The video player ([SessionPlayback.videoPlayer]) of one work: the core's
 * [PlayController] over the app's [engine], started at once with this
 * device's capability report, qualities and dialogue boost, from the
 * server, or from a download's files with no server, which has nothing to
 * fall back to and no download rate to measure; stopped with a message
 * when the live events say its play ended elsewhere. Around the play, its
 * [screen]: the work and its next episode, the qualities, the scrub
 * previews, what shows over the video by the timed rules every player
 * keeps, from the app's inputs, and where the player goes once it's done.
 * The app follows [controller]'s state and drives its transport (pause,
 * seek, retry and the like) directly.
 */
class VideoPlayer internal constructor(
    private val session: KivoSession,
    private val engine: PlayerEngine,
    private val choice: PlayChoice,
    private val downloadId: String?,
    positionMs: Long?,
    hideControlsAfter: Duration,
) {
    private val core = session.core
    private val online = downloadId == null
    private val job = SupervisorJob(session.scope.coroutineContext[Job])
    private val scope = CoroutineScope(session.scope.coroutineContext + job)

    private val mutableScreen = MutableStateFlow(PlayerScreenState(offline = !online))

    /** What the screen shows around the video; it changes as the work, the play's version or what shows over the video does, not with the position. */
    val screen: StateFlow<PlayerScreenState> = mutableScreen.asStateFlow()

    private var ladder = emptyList<QualityRung>()

    /** The download's play once it's open. */
    private val opened = CompletableDeferred<OfflinePlay>()

    val controller: PlayController = PlayController(
        plays = if (online) ServerPlays(session.connection.plays, session.connection.reachability) else DownloadPlays(opened),
        engine = engine,
        capabilities = { external -> core.report.report(external) },
        scope = core.scope,
        canFallBack = online,
        onEnded = { rules.ended() },
    )

    private val rules: PlayerUiRules = PlayerUiRules(
        controller.state, controller::seek, core.autoplay, session.scope, core.clock, hideControlsAfter,
        changed = { ui -> mutableScreen.value = mutableScreen.value.copy(ui = ui) },
    )

    /** Whether up next was told of a next episode, once it was told anything. */
    private var toldNext: Boolean? = null

    init {
        scope.launch { start(positionMs) }
        scope.launch { session.playsEnded.collect { controller.playEnded(it.playId, it.message) } }
        // A stream playing holds the server's address where it is while it answers.
        if (online) {
            scope.launch {
                controller.state.map { it.live && !it.paused }.distinctUntilChanged()
                    .collect { session.connection.address.streaming(this@VideoPlayer, it) }
            }
        }
        scope.launch { followWork() }
        scope.launch {
            // Without the ladder, Quality offers Auto and Original.
            ladder = quietly { session.playback.qualityLadder() }.orEmpty()
            changeVersion()
        }
        scope.launch { controller.state.map { it.play?.versionId }.distinctUntilChanged().collect { changeVersion() } }
        scope.launch {
            // Without previews, the scrub bar shows the time alone.
            controller.state.map { it.play }.distinctUntilChangedBy { it?.previews }.collectLatest { play ->
                change { it.copy(previews = null) }
                val previews = play?.let { quietly { session.playback.previews(it) } } ?: return@collectLatest
                change { it.copy(previews = previews) }
            }
        }
        scope.launch {
            rules.end.collect { end ->
                val exit = end?.let { exitAt(it) } ?: return@collect
                change { it.copy(exit = exit) }
            }
        }
    }

    /** Opens the download, when it's one, and starts the play; a download that can't play now, such as one just removed, goes back. */
    private suspend fun start(positionMs: Long?) {
        if (downloadId != null) {
            val offline = quietly { checkNotNull(core.downloads) { "this device keeps no downloads" }.offline(downloadId) }
                ?: return change { it.copy(exit = PlayerExit.Back) }
            opened.complete(offline)
            change { it.copy(downloadSource = offline.source()) }
        }
        controller.play(request(choice, positionMs))
    }

    /**
     * The work as it changes, and its next episode, read again when the next
     * one changes or wasn't read: up next and the end follow whether it was
     * read, which without the server means whether it's downloaded.
     */
    private suspend fun followWork() {
        session.playback.playableWork(choice.workId).collect { load ->
            val work = load.getOrNull() ?: return@collect
            change { it.copy(work = work) }
            changeVersion()
            val episode = work.episode ?: return@collect
            val nextId = episode.nextId
            val next = nextId?.let { id -> screen.value.next?.takeIf { it.id == id } ?: session.playback.playableWork(id).answer() }
            change { it.copy(next = next) }
            if (toldNext != (next != null)) {
                toldNext = next != null
                setHasNextEpisode(next != null)
            }
        }
    }

    /** The version playing and its qualities, from the work, the play and the ladder. */
    private fun changeVersion() {
        val versionId = controller.state.value.play?.versionId
        val version = screen.value.work?.versions?.firstOrNull { it.id == versionId }
        val sourceBps = version?.bitrateBps
        // Only rungs below the file's own bitrate change anything.
        val qualities = qualityMenu(ladder.filter { sourceBps == null || it.bitrateBps < sourceBps })
        change { it.copy(version = version, qualities = qualities) }
    }

    /**
     * Where the core's [end] takes the player: the next episode, from its
     * download when it's ready here, the series or back. A download goes
     * back to where it was played from, since its series' page needs the server.
     */
    private suspend fun exitAt(end: PlayerEnd): PlayerExit? = when (end) {
        PlayerEnd.PlayNext -> screen.value.next?.let { PlayerExit.Next(PlayChoice(it.id), readyDownload(it.id)) }
        PlayerEnd.Series -> if (!online) PlayerExit.Back else screen.value.work?.episode?.let { PlayerExit.Series(it.seriesId) }
        PlayerEnd.Close -> PlayerExit.Back
    }

    private suspend fun readyDownload(workId: String): String? =
        session.downloads?.items?.first()?.firstOrNull { it.workId == workId && it.status == DownloadStatus.Ready }?.id

    private inline fun change(update: (PlayerScreenState) -> PlayerScreenState) {
        mutableScreen.value = update(mutableScreen.value)
    }

    /**
     * This video is an episode, with a next one to play or not: up next and
     * the end follow it, and the user's autoplay, read again from the server
     * when there's a next one.
     */
    internal fun setHasNextEpisode(hasNext: Boolean) {
        if (online && hasNext) session.account.readAutoplay()
        rules.episode(hasNext)
    }

    /** Any input: someone is watching, so up next may play on again, and the controls wait again to hide. */
    fun userActivity() = rules.input()

    /** Input that shows the controls, such as a tap, a mouse move or a key. */
    fun showControls() = rules.showControls()

    /** Hides the controls, as Back does; they also hide by themselves a while after the last input while playing, with no menu open or scrub held. */
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

    /** Up next's Play now, or Next: [PlayerScreenState.exit] becomes the next episode. */
    fun playNext() = rules.playNext()

    /** The play [start] starts. A null [positionMs] starts at the resume position, 0 at the start. */
    internal suspend fun request(choice: PlayChoice, positionMs: Long? = null): PlayRequest {
        val settings = if (online) session.playback.playSettings() else core.settingsNow()
        val auto = settings.localQuality == Quality.Auto || settings.remoteQuality == Quality.Auto
        val bandwidthBps = if (online && auto) core.bandwidth.forAuto(session.connection) else null
        return PlayRequest(
            workId = choice.workId,
            versionId = choice.versionId,
            audioTrackId = choice.audioTrackId,
            subtitleTrackId = choice.subtitleTrackId,
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
     * Adds the subtitle search's result [resultId] to the version playing on
     * the server and shows it: the version's tracks gain it at once, before
     * the work is read again with it.
     */
    @Throws(Exception::class)
    suspend fun addSubtitle(resultId: String) {
        val version = screen.value.version ?: return
        val track = session.playback.addSubtitle(version, resultId)
        change { it.copy(work = it.work?.withTrack(version.id, track)) }
        changeVersion()
        chooseSubtitle(track.id)
    }

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
        job.cancel()
        if (online) session.connection.address.streaming(this, false)
        rules.dispose()
    }
}

/** A download's plays, once [opened]. */
private class DownloadPlays(private val opened: CompletableDeferred<OfflinePlay>) : PlaySource {
    override suspend fun start(request: PlayCreate): PlayStarted = opened.await().plays.start(request)

    override suspend fun heartbeat(id: String, heartbeat: PlayUpdate) = opened.await().plays.heartbeat(id, heartbeat)

    override suspend fun reconnect(id: String, heartbeat: PlayUpdate) = opened.await().plays.reconnect(id, heartbeat)

    override suspend fun stop(id: String, stop: PlayStop) = opened.await().plays.stop(id, stop)

    override suspend fun fallback(id: String, request: PlayFallback): PlayStarted = opened.await().plays.fallback(id, request)
}
