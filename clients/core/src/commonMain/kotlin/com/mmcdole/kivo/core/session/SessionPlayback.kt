package com.mmcdole.kivo.core.session

import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.PreviewManifest
import com.mmcdole.kivo.api.models.QualityRung
import com.mmcdole.kivo.api.models.SubtitleResult
import com.mmcdole.kivo.api.models.Track
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.books.BookAudio
import com.mmcdole.kivo.core.books.BookPlayerModel
import com.mmcdole.kivo.core.books.openBook
import com.mmcdole.kivo.core.data.SubtitleSearcher
import com.mmcdole.kivo.core.display.QualityOption
import com.mmcdole.kivo.core.display.qualityMenu
import com.mmcdole.kivo.core.play.ServerPlays
import com.mmcdole.kivo.core.player.PlayerEngine
import com.mmcdole.kivo.core.settings.DeviceSettings
import io.ktor.client.call.body
import io.ktor.client.request.get
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.emitAll
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * Playing on this device ([KivoSession.playback]): the video player and the
 * book player, each started with this device's settings, the server's
 * quality ladder, a play's scrub previews, and the subtitle search the
 * player offers.
 */
class SessionPlayback internal constructor(private val session: KivoSession) {
    private val connection = session.connection
    private val core = session.core
    private val subtitles = SubtitleSearcher(connection)

    /**
     * A work for the player: as it was downloaded at once, when it's
     * downloaded, so it shows without the server; then the server's, as
     * it answers and as it changes.
     */
    fun playableWork(id: String): Flow<Load<WorkDetail>> = flow {
        core.downloads?.storedWork(id)?.let { emit(Load.Loaded(it)) }
        emitAll(session.library.work(id))
    }

    /** The subtitle search's results for [version]'s file, best first; a failure's `SubtitleSearch.problem` says why. */
    @Throws(Exception::class)
    suspend fun subtitleResults(version: Version): List<SubtitleResult> = subtitles.results(version)

    /** Adds a subtitle result to [version]'s file on the server; the track it adds. */
    @Throws(Exception::class)
    suspend fun addSubtitle(version: Version, resultId: String): Track = subtitles.add(version, resultId)

    private val rungsLock = Mutex()
    private var rungs: List<QualityRung>? = null

    /** The server's quality ladder, highest first, read once while the session is open. */
    @Throws(Exception::class)
    suspend fun qualityLadder(): List<QualityRung> = rungsLock.withLock {
        rungs ?: session.serverInfo().qualityLadder.orEmpty().also { rungs = it }
    }

    /** Auto, Original and the server's ladder. */
    @Throws(Exception::class)
    suspend fun qualityMenu(): List<QualityOption> = qualityMenu(qualityLadder())

    /**
     * This device's settings for a play of this server's: a quality the
     * server's ladder doesn't have is Auto; as they are when the ladder
     * can't be read.
     */
    internal suspend fun playSettings(): DeviceSettings {
        val settings = core.settingsNow()
        return try {
            settings.onLadder(qualityLadder())
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            settings
        }
    }

    /** The play's preview thumbnails once they're ready; null while they aren't, and the bar shows the time alone. */
    @Throws(Exception::class)
    suspend fun previews(play: PlayStarted): ScrubPreviews? {
        val relative = play.previews?.takeIf { it.ready }?.url ?: return null
        val url = session.mediaUrl(relative)
        return ScrubPreviews(url, connection.http.get(url).body())
    }

    /** A video player on [engine] for this server. A video stops the book playing, keeping its place. */
    fun videoPlayer(engine: PlayerEngine): VideoPlayer {
        stopBook()
        return VideoPlayer(session, engine, ServerPlays(connection.plays, connection.reachability), offline = null)
    }

    /**
     * A player for a download, from its files with no server, and the work as
     * it was downloaded. A video stops the book playing, keeping its place.
     */
    @Throws(Exception::class)
    suspend fun offlinePlayer(engine: PlayerEngine, downloadId: String): VideoPlayer {
        val downloads = checkNotNull(core.downloads) { "this device keeps no downloads" }
        stopBook()
        val offline = downloads.offline(downloadId)
        return VideoPlayer(session, engine, offline.plays, offline)
    }

    /**
     * Someone is watching: an input outside a player, such as a cast's Try
     * again, starts up next's run of episodes played on without input again,
     * as any input in a player does.
     */
    fun confirmStillWatching() {
        core.autoplay.reset()
    }

    private var books: BookPlayerModel? = null

    /**
     * The book player, one for the session, on the app's audio [engine] and
     * [audio]: each book from its download here when one is ready, else
     * streamed. It stops when its play ends elsewhere, when a video starts,
     * and as the session closes.
     */
    fun bookPlayer(engine: PlayerEngine, audio: BookAudio): BookPlayerModel {
        books?.let { return it }
        val model = BookPlayerModel(
            engine = engine,
            audio = audio,
            open = { openBook(it, connection, core.downloads) },
            capabilities = { core.report.report() },
            userSettings = session.userSettings,
            scope = core.scope,
        )
        session.scope.launch { session.playsEnded.collect { model.playEnded(it.playId, it.message) } }
        session.scope.launch {
            model.state.map { !it.downloaded && it.player.live && !it.player.paused }.distinctUntilChanged()
                .collect { connection.address.streaming(model, it) }
        }
        books = model
        return model
    }

    /** The book playing stops, keeping its place, as a video starts or the session closes. */
    internal fun stopBook() {
        books?.stop()
    }
}

/** A play's scrub previews: the manifest, and its URL, which its sheets resolve against. */
data class ScrubPreviews(val url: String, val manifest: PreviewManifest)
