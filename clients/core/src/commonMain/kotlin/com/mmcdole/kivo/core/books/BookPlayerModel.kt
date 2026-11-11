package com.mmcdole.kivo.core.books

import com.mmcdole.kivo.api.models.Capabilities
import com.mmcdole.kivo.api.models.PlaybackPreferencesUpdate
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.downloads.Downloads
import com.mmcdole.kivo.core.player.SKIP_BACK_MS
import com.mmcdole.kivo.core.player.SKIP_FORWARD_MS
import com.mmcdole.kivo.core.player.PlayerEngine
import com.mmcdole.kivo.core.play.PlayRequest
import com.mmcdole.kivo.core.play.PlayController
import com.mmcdole.kivo.core.play.PlaySource
import com.mmcdole.kivo.core.play.PlayerFailure
import com.mmcdole.kivo.core.play.PlayerState
import com.mmcdole.kivo.core.play.PlayerStatus
import com.mmcdole.kivo.core.play.ServerPlays
import com.mmcdole.kivo.core.play.failureOf
import com.mmcdole.kivo.core.settings.UserSettings
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlin.time.TimeSource

/**
 * The book player: one per signed-in server, a [PlayController] for each book
 * it plays, over the platform's [engine], so it keeps playing while the app
 * browses. It opens a book through [open], from a download or the server, and
 * keeps what the mini player, Now Playing and the lock screen show: the
 * chapter playing and where in it, the whole book's progress and time left,
 * the user's speed and the sleep timer. Its commands are theirs: play and
 * pause, 10 seconds back and 30 forward, a seek within the chapter, previous
 * chapter with the 3-second rule, next chapter, a chapter from the list, the
 * speed, which it saves for the user on every device, the sleep timer, and
 * stop, which saves the place, as a video starting does.
 *
 * Call it on one thread, [scope]'s, as the controller.
 */
class BookPlayerModel internal constructor(
    private val engine: PlayerEngine,
    private val audio: BookAudio,
    private val open: suspend (workId: String) -> BookSource,
    private val capabilities: suspend () -> Capabilities,
    private val userSettings: UserSettings,
    private val scope: CoroutineScope,
    private val now: () -> Long = TimeSource.Monotonic.markNow().let { start -> { start.elapsedNow().inWholeMilliseconds } },
) {
    private val mutableState = MutableStateFlow(BookPlayerState())
    val state: StateFlow<BookPlayerState> = mutableState.asStateFlow()

    private var controller: PlayController? = null
    private var loading: Job? = null
    private var chaptersOf: String? = null
    private val sleep = SleepTimer()
    private var sleeping: Job? = null

    /** Where End of chapter pauses: the end of the chapter playing when it was set or last moved. */
    private var chapterEndMs = 0L
    private var speedRead = false
    private var speedChosen = false

    /** The last sleep timer chosen, which the menu offers first. */
    val lastSleep: SleepChoice? get() = sleep.last

    /**
     * Plays a book: from where the user left it, or from [positionMs] on
     * its timeline, such as a chapter's start. The book already loaded
     * seeks there and plays on.
     */
    fun play(workId: String, positionMs: Long? = null) {
        val loaded = controller
        if (state.value.workId == workId && loaded?.current != null) {
            positionMs?.let { seek(it) }
            loaded.resume()
            return
        }
        close()
        mutableState.value = BookPlayerState(
            workId = workId,
            player = PlayerState(status = PlayerStatus.Loading, positionMs = positionMs ?: 0),
            speed = state.value.speed,
        )
        readSpeed()
        loading = scope.launch {
            val source = try {
                open(workId)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                mutableState.update { it.copy(player = it.player.copy(status = PlayerStatus.Error), failure = failureOf(e)) }
                return@launch
            }
            val version = source.work.versions.firstOrNull()
            val position = positionMs ?: source.work.userState?.positionMs?.toLong() ?: 0
            mutableState.update { s ->
                val chapters = version?.let { BookChapters(it) }
                s.copy(work = source.work, downloaded = source.downloaded, chapters = chapters, progress = chapters?.progress(position))
            }
            val controller = PlayController(
                plays = source.plays,
                engine = engine,
                capabilities = { capabilities() },
                scope = scope,
                canFallBack = false,
                onEnded = ::ended,
            )
            this@BookPlayerModel.controller = controller
            audio.setSpeed(state.value.speed)
            controller.play(PlayRequest(workId, positionMs = positionMs))
            controller.state.collect(::follow)
        }
    }

    fun togglePlay() {
        val controller = controller ?: return
        if (state.value.player.status == PlayerStatus.Error) controller.retry() else controller.togglePlay()
    }

    fun resume() {
        controller?.resume()
    }

    fun pause() {
        controller?.pause()
    }

    fun skipBack() = seek(position() - SKIP_BACK_MS)

    fun skipForward() = seek(position() + SKIP_FORWARD_MS)

    /** Moves to a position on the book's timeline. */
    fun seek(positionMs: Long) {
        val controller = controller ?: return
        controller.seek(positionMs)
        aimSleep(positionMs.coerceIn(0, state.value.chapters?.durationMs ?: positionMs))
    }

    /** The seek bar's seek on release, [offsetMs] into the chapter at [index], the one it spanned. */
    fun seekInChapter(index: Int, offsetMs: Long) {
        val chapters = state.value.chapters ?: return
        val start = chapters.startMs(index)
        seek(start + offsetMs.coerceIn(0, chapters.endMs(index) - start))
    }

    /** Back to the chapter's start, or to the chapter before's within 3 seconds of it. */
    fun previousChapter() {
        val chapters = state.value.chapters ?: return
        seek(chapters.previousMs(position()))
    }

    /** To the next chapter's start; none in the last. */
    fun nextChapter() {
        val chapters = state.value.chapters ?: return
        chapters.nextMs(position())?.let(::seek)
    }

    /** Plays a chapter from the list, from its start. */
    fun playChapter(index: Int) {
        val chapters = state.value.chapters ?: return
        seek(chapters.startMs(index))
        resume()
    }

    /** Sets the speed here at once and for the user on every device, shared with the web. */
    fun setSpeed(speed: Double) {
        speedChosen = true
        mutableState.update { it.copy(speed = speed) }
        audio.setSpeed(speed)
        scope.launch {
            try {
                userSettings.updatePlayback(PlaybackPreferencesUpdate(bookSpeed = speed))
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // Offline: it plays at this speed here, and the user keeps its last.
            }
        }
    }

    /** Starts the sleep timer afresh. */
    fun setSleep(choice: SleepChoice) {
        aimSleep(position(), choice)
        apply(sleep.set(choice, chapterEndMs - position(), state.value.speed))
        publishSleep()
        if (sleeping?.isActive != true) {
            sleeping = scope.launch {
                var last = now()
                while (sleep.state is SleepState.Running) {
                    delay(SLEEP_TICK_MS)
                    val at = now()
                    val running = sleep.state as? SleepState.Running ?: break
                    val s = state.value
                    val playing = !s.player.paused && s.player.status == PlayerStatus.Playing
                    val command = sleep.tick(at - last, playing, chapterEndMs - position(), s.speed)
                    last = at
                    if (command == SleepCommand.Pause && running.choice == SleepChoice.EndOfChapter) {
                        controller?.pause()
                        if (chapterEndMs < (s.chapters?.durationMs ?: 0)) controller?.seek(chapterEndMs)
                        audio.restoreVolume()
                    } else {
                        apply(command)
                    }
                    publishSleep()
                }
            }
        }
    }

    /** Turns the sleep timer off. */
    fun stopSleep() {
        apply(sleep.off())
        publishSleep()
    }

    /** A play ended elsewhere (the live events' `playEnded`): if it's the book's, it stops and says so, with the admin's [message] if there is one. */
    fun playEnded(playId: String, message: String? = null) {
        controller?.playEnded(playId, message)
    }

    /** Stops the book, saving its place, and unloads it: the mini player goes. A video starting does this. */
    fun stop() {
        close()
        mutableState.value = BookPlayerState(speed = state.value.speed)
    }

    private fun close() {
        loading?.cancel()
        loading = null
        controller?.dispose()
        controller = null
        chaptersOf = null
        apply(sleep.off())
        sleeping?.cancel()
        sleeping = null
    }

    private fun position(): Long = controller?.positionMs() ?: state.value.player.positionMs

    private fun follow(player: PlayerState) {
        mutableState.update { s ->
            val play = player.play
            val chapters = if (play != null && play.id != chaptersOf) {
                chaptersOf = play.id
                BookChapters(play)
            } else {
                s.chapters
            }
            s.copy(player = player, chapters = chapters, progress = chapters?.progress(player.positionMs), failure = player.failure)
        }
    }

    /** The book played to its end: the sleep timer has nothing left to do. */
    private fun ended() {
        apply(sleep.off())
        publishSleep()
    }

    /** End of chapter follows the position to the chapter it's moved to. */
    private fun aimSleep(positionMs: Long, choice: SleepChoice? = (sleep.state as? SleepState.Running)?.choice) {
        if (choice != SleepChoice.EndOfChapter) return
        val chapters = state.value.chapters ?: return
        chapterEndMs = chapters.endMs(chapters.indexAt(positionMs))
    }

    private fun apply(command: SleepCommand?) {
        when (command) {
            is SleepCommand.FadeOut -> audio.fadeOut(command.durationMs)
            SleepCommand.RestoreVolume -> audio.restoreVolume()
            SleepCommand.Pause -> {
                controller?.pause()
                audio.restoreVolume()
            }
            null -> Unit
        }
    }

    private fun publishSleep() {
        mutableState.update { it.copy(sleep = sleep.state) }
    }

    /** The user's speed, read once, unless one is chosen here first. */
    private fun readSpeed() {
        if (speedRead) return
        speedRead = true
        scope.launch {
            try {
                val speed = userSettings.current().playback.bookSpeed
                if (!speedChosen) {
                    mutableState.update { it.copy(speed = speed) }
                    if (controller != null) audio.setSpeed(speed)
                }
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                speedRead = false
            }
        }
    }

    companion object {
        /** The speeds a book plays at, 1 being normal. */
        val SPEEDS: List<Double> = listOf(0.5, 0.75, 0.8, 0.9, 1.0, 1.1, 1.2, 1.25, 1.3, 1.4, 1.5, 1.6, 1.75, 2.0, 2.25, 2.5, 2.75, 3.0)
    }
}

/** The book player's state: nothing loaded when [workId] is null, and no mini player. */
data class BookPlayerState(
    val workId: String? = null,
    /** The book, once it's known. */
    val work: WorkDetail? = null,
    /** Playing its download here rather than streaming it. */
    val downloaded: Boolean = false,
    val player: PlayerState = PlayerState(),
    /** The chapters, from the play once it starts, from the book's version before. */
    val chapters: BookChapters? = null,
    val progress: BookProgress? = null,
    /** The user's speed for books. */
    val speed: Double = 1.0,
    val sleep: SleepState = SleepState.Off,
    /** Why it isn't playing: the play's failure, or the book couldn't be opened. */
    val failure: PlayerFailure? = null,
) {
    val playing: Boolean get() = !player.paused
}

/**
 * What the book player needs of the platform's audio beyond [PlayerEngine]:
 * the speed, keeping the pitch, and the sleep timer's fade.
 */
interface BookAudio {
    fun setSpeed(speed: Double)

    /** Lowers the volume to silence over [durationMs]. */
    fun fadeOut(durationMs: Long)

    /** Full volume again, at once. */
    fun restoreVolume()
}

/** Where a book plays from: the work to show, and its plays, a download's here or the server's. */
internal class BookSource(val work: WorkDetail, val plays: PlaySource, val downloaded: Boolean)

/**
 * A book from its download on this device when one is ready to play
 * ([Downloads.playable]), else streamed through the server's plays,
 * as the web plays it.
 */
internal suspend fun openBook(workId: String, connection: ServerConnection, downloads: Downloads?): BookSource {
    downloads?.playable(workId)?.let { return BookSource(it.work, it.plays, downloaded = true) }
    val work = connection.works.getWork(workId).result()
    return BookSource(work, ServerPlays(connection.plays, connection.reachability), downloaded = false)
}

/** How often the sleep timer is checked while it runs. */
internal const val SLEEP_TICK_MS = 250L
