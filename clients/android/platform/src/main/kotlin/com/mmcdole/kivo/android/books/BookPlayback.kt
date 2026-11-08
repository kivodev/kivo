package com.mmcdole.kivo.android.books

import androidx.media3.common.MediaMetadata
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.core.books.BookPlayerModel
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.job
import kotlinx.coroutines.launch

/**
 * The signed-in server's book player on the phone: the core's
 * [BookPlayerModel] over [Media3BookEngine], whose player the app's media
 * session controls, so a book plays on in the background with the
 * notification and lock screen. The session's previous and next move by
 * chapter, and it shows the book's title, author and cover.
 */
class BookPlayback(session: ServerSession) {
    private val scope = CoroutineScope(session.graph.scope.coroutineContext + SupervisorJob(session.graph.scope.coroutineContext.job))

    private val engine = Media3BookEngine(session.graph.context, session.session::mediaUrl, session.graph.scope, session.graph.sessionPlayers)

    val model: BookPlayerModel = session.session.playback.bookPlayer(engine, engine)

    init {
        engine.previous = model::previousChapter
        engine.next = model::nextChapter
        scope.launch {
            model.state.map { it.work }.distinctUntilChanged { a, b -> a?.id == b?.id }.collect { work ->
                engine.metadata = work?.let(::metadataOf) ?: MediaMetadata.EMPTY
                val cover = work?.images?.poster ?: return@collect
                val bytes = try {
                    session.session.bytes(session.session.imageUrl(cover.id, COVER_PX))
                } catch (e: CancellationException) {
                    throw e
                } catch (_: Exception) {
                    // The notification shows the app's icon instead.
                    return@collect
                }
                engine.metadata = metadataOf(work).buildUpon().setArtworkData(bytes, MediaMetadata.PICTURE_TYPE_FRONT_COVER).build()
            }
        }
    }

    /** The book stops, saving its place, and the session lets go of it; the server is closing. */
    fun close() {
        scope.cancel()
        model.stop()
    }
}

/** The book's title and its authors. */
private fun metadataOf(work: WorkDetail): MediaMetadata = MediaMetadata.Builder()
    .setTitle(work.title)
    .setArtist(work.book?.authors?.joinToString(", ") { it.name })
    .build()

/** The cover's width for the notification and lock screen. */
private const val COVER_PX = 480
