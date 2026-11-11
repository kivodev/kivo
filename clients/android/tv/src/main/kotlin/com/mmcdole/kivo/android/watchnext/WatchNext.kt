package com.mmcdole.kivo.android.watchnext

import android.annotation.SuppressLint
import android.content.ContentProvider
import android.content.ContentValues
import android.content.Context
import android.net.Uri
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.core.net.toUri
import androidx.tvprovider.media.tv.TvContractCompat
import androidx.tvprovider.media.tv.TvContractCompat.WatchNextPrograms
import androidx.tvprovider.media.tv.WatchNextProgram
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.ExistingWorkPolicy
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.mmcdole.kivo.android.AppState
import com.mmcdole.kivo.android.HasAppGraph
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.api.models.HomeRowKind
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.Load
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withTimeoutOrNull
import java.io.File
import java.util.concurrent.TimeUnit
import kotlin.time.Duration.Companion.seconds

/**
 * Continue Watching in the Watch Next row of the Android TV and Google TV
 * home screen: read from
 * the server every hour, and soon after a video closes; each opens its
 * detail page. Removing one from the row leaves it out until it leaves the
 * server's rows. Fire TV has no Watch Next and nothing is written there.
 *
 * Lint takes tvprovider's program builders, public API inherited from a
 * library-internal base class, for restricted API.
 */
@SuppressLint("RestrictedApi")
object WatchNext {
    private const val PERIODIC = "watchNext"
    private const val SOON = "watchNextSoon"

    /** Keeps the row up to date while the app is installed. */
    fun schedule(context: Context) {
        WorkManager.getInstance(context).enqueueUniquePeriodicWork(
            PERIODIC,
            ExistingPeriodicWorkPolicy.KEEP,
            PeriodicWorkRequestBuilder<WatchNextWorker>(1, TimeUnit.HOURS).build(),
        )
    }

    /** Reads the rows again in a few seconds, once a closing player's position has reached the server. */
    fun refresh(context: Context) {
        WorkManager.getInstance(context).enqueueUniqueWork(
            SOON,
            ExistingWorkPolicy.REPLACE,
            OneTimeWorkRequestBuilder<WatchNextWorker>().setInitialDelay(5, TimeUnit.SECONDS).build(),
        )
    }

    /** The link a program opens: the work's detail page. */
    fun link(workId: String): Uri = "kivo://work/$workId".toUri()

    /** The work a link names, or null for anything else. */
    fun workOf(uri: Uri?): String? = uri?.takeIf { it.scheme == "kivo" && it.host == "work" }?.lastPathSegment

    /** Puts [session]'s Continue Watching in the row, in its order, and takes out what's no longer in it. */
    suspend fun publish(context: Context, session: ServerSession) {
        val read = session.session.library.home(HomeRowKind.ContinueWatching).first { it !is Load.Loading }
        // The server didn't answer: the row stays as it was until the next refresh.
        val home = read.getOrNull() ?: return
        // A work in progress continues; a series' next episode is next.
        val wanted = home.rows.filter { it.kind == HomeRowKind.ContinueWatching }.flatMap { it.items }.map {
            it to if (it.userState?.positionMs != null) WatchNextPrograms.WATCH_NEXT_TYPE_CONTINUE else WatchNextPrograms.WATCH_NEXT_TYPE_NEXT
        }
        val unique = wanted.distinctBy { it.first.id }
        val existing = programs(context)
        val now = System.currentTimeMillis()
        val art = ArtworkProvider.directory(context)
        val kept = HashSet<String>()
        for ((index, entry) in unique.withIndex()) {
            val (work, type) = entry
            val old = existing[work.id]
            // Taken out of the row by the viewer: left out until it leaves the server's rows.
            if (old != null && !old.isBrowsable) continue
            val image = (if (type == WatchNextPrograms.WATCH_NEXT_TYPE_CONTINUE) work.images.still ?: work.images.backdrop else null) ?: work.images.poster
            val poster = image?.let { ref ->
                kept += ref.id
                val file = File(art, ref.id)
                if (!file.exists()) {
                    try {
                        file.writeBytes(session.session.bytes(session.session.imageUrl(ref.id, ARTWORK_WIDTH)))
                    } catch (e: CancellationException) {
                        throw e
                    } catch (_: Exception) {
                        // Without its artwork this time; fetched again at the next refresh.
                    }
                }
                ArtworkProvider.uri(context, ref.id).takeIf { file.exists() }
            }
            val program = program(work, type, poster, landscape = image != null && image != work.images.poster, engaged = now - index * 1000L)
            if (old == null) {
                context.contentResolver.insert(WatchNextPrograms.CONTENT_URI, program.toContentValues())
            } else {
                context.contentResolver.update(TvContractCompat.buildWatchNextProgramUri(old.id), program.toContentValues(), null, null)
            }
        }
        for ((workId, old) in existing) {
            if (unique.none { it.first.id == workId }) context.contentResolver.delete(TvContractCompat.buildWatchNextProgramUri(old.id), null, null)
        }
        art.listFiles()?.filter { it.name !in kept }?.forEach { it.delete() }
    }

    /** Takes everything out of the row, as the app leaves its server. */
    fun clear(context: Context) {
        // Without a TV provider (Fire TV) there's no row to empty.
        runCatching {
            for (old in programs(context).values) context.contentResolver.delete(TvContractCompat.buildWatchNextProgramUri(old.id), null, null)
        }
        ArtworkProvider.directory(context).listFiles()?.forEach { it.delete() }
    }

    /** This app's programs in the row, by the work they're for. */
    private fun programs(context: Context): Map<String, WatchNextProgram> = buildMap {
        context.contentResolver.query(WatchNextPrograms.CONTENT_URI, null, null, null, null)?.use { cursor ->
            while (cursor.moveToNext()) {
                val program = WatchNextProgram.fromCursor(cursor)
                program.internalProviderId?.let { put(it, program) }
            }
        }
    }

    private fun program(work: WorkSummary, type: Int, poster: Uri?, landscape: Boolean, engaged: Long): WatchNextProgram {
        val episode = work.episode
        val builder = WatchNextProgram.Builder()
            .setWatchNextType(type)
            .setLastEngagementTimeUtcMillis(engaged)
            .setInternalProviderId(work.id)
            .setIntentUri(link(work.id))
            .setTitle(episode?.seriesTitle ?: work.title)
        builder.setType(
            when (work.kind) {
                WorkKind.Episode -> WatchNextPrograms.TYPE_TV_EPISODE
                WorkKind.Movie -> WatchNextPrograms.TYPE_MOVIE
                else -> WatchNextPrograms.TYPE_CLIP
            },
        )
        if (episode != null) {
            builder.setEpisodeTitle(work.title)
                .setSeasonNumber(episode.seasonNumber)
                .setEpisodeNumber(episode.number)
        }
        work.durationMs?.let { builder.setDurationMillis(it) }
        work.userState?.positionMs?.let { builder.setLastPlaybackPositionMillis(it) }
        poster?.let {
            builder.setPosterArtUri(it)
                .setPosterArtAspectRatio(if (landscape) WatchNextPrograms.ASPECT_RATIO_16_9 else WatchNextPrograms.ASPECT_RATIO_2_3)
        }
        return builder.build()
    }

    private const val ARTWORK_WIDTH = 480
}

/** Refreshes the row once the app has reached its server; signed out, it empties the row. */
class WatchNextWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val graph = (applicationContext as HasAppGraph).graph
        val state = withTimeoutOrNull(30.seconds) { graph.state.first { it !is AppState.Starting } } ?: return Result.success()
        try {
            when (state) {
                is AppState.SignedIn -> WatchNext.publish(applicationContext, state.session)
                is AppState.SigningIn -> WatchNext.clear(applicationContext)
                else -> Unit // The server can't be reached: the row stays as it was until the next try.
            }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            // No TV provider (Fire TV), or the server didn't answer: tried again at the next refresh.
            Log.w("WatchNext", "Watch Next not refreshed", e)
        }
        return Result.success()
    }
}

/**
 * The Watch Next row's artwork, which the launcher reads itself: the
 * server's images need the app session's token, so each is kept in the app's
 * cache as it's published and served from there.
 */
class ArtworkProvider : ContentProvider() {
    override fun onCreate() = true

    override fun openFile(uri: Uri, mode: String): ParcelFileDescriptor? {
        val context = context ?: return null
        val name = uri.lastPathSegment?.takeIf { it.isNotEmpty() && '/' !in it && !it.startsWith(".") } ?: return null
        val file = File(directory(context), name)
        return if (file.exists()) ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY) else null
    }

    override fun getType(uri: Uri) = "image/webp"

    override fun query(uri: Uri, projection: Array<out String>?, selection: String?, selectionArgs: Array<out String>?, sortOrder: String?) = null

    override fun insert(uri: Uri, values: ContentValues?) = null

    override fun delete(uri: Uri, selection: String?, selectionArgs: Array<out String>?) = 0

    override fun update(uri: Uri, values: ContentValues?, selection: String?, selectionArgs: Array<out String>?) = 0

    companion object {
        fun directory(context: Context) = File(context.cacheDir, "watchnext").apply { mkdirs() }

        fun uri(context: Context, imageId: String): Uri = "content://${context.packageName}.artwork/$imageId".toUri()
    }
}
