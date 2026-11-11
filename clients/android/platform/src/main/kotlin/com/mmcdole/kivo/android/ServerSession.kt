package com.mmcdole.kivo.android

import coil3.ImageLoader
import coil3.disk.DiskCache
import coil3.network.ktor3.KtorNetworkFetcherFactory
import com.mmcdole.kivo.android.books.BookPlayback
import com.mmcdole.kivo.android.library.GridChoice
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.core.downloads.DownloadItem
import com.mmcdole.kivo.core.downloads.DownloadStatus
import com.mmcdole.kivo.core.session.KivoSession
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import okio.Path.Companion.toOkioPath
import java.io.File

/**
 * The signed-in server as the app's screens keep it: the core's [session]
 * session, and what the screens keep beside it while it's open, the image
 * loader, the library grids' choices, the last search and a phone's downloads. Made when the app signs in or reaches its server,
 * and dropped on sign-out.
 */
class ServerSession(val graph: AppGraph, val session: KivoSession, savedGrids: Map<String, GridChoice>) {
    /** Work that lasts while this server is open, on the app's scope. */
    private val scope = CoroutineScope(graph.scope.coroutineContext + SupervisorJob(graph.scope.coroutineContext.job))

    /**
     * The user's libraries, shared by the screens that show them, so a
     * library's screen has its name and kind as it opens. Null until read,
     * or while a read has failed.
     */
    val libraryList: StateFlow<List<Library>?> = session.library.libraries()
        .map { it.getOrNull() }
        .stateIn(graph.scope, SharingStarted.WhileSubscribed(5_000), null)

    /** The last search typed, which Search opens on again. */
    var lastSearch = ""

    /**
     * The library grids' choices as this device saved them, at hand as a
     * library opens: [savedGrids] as the server opened, then each save.
     */
    val gridChoices: StateFlow<Map<String, GridChoice>> =
        graph.savedGrids.all.stateIn(scope, SharingStarted.Eagerly, savedGrids)

    /** Artwork through the session, so it carries its token; cached on disk per server. */
    val images: ImageLoader = ImageLoader.Builder(graph.context)
        .components { add(KtorNetworkFetcherFactory(httpClient = { session.http })) }
        .diskCache {
            DiskCache.Builder()
                .directory(File(graph.context.cacheDir, "images/${session.savedServer.value.id}").toOkioPath())
                .maxSizeBytes(256L * 1024 * 1024)
                .build()
        }
        .build()

    /** This server's downloads on the device, newest first, kept while it's open; none on a TV. */
    val downloads: StateFlow<List<DownloadItem>> =
        session.downloads?.items?.stateIn(scope, SharingStarted.Eagerly, emptyList()) ?: MutableStateFlow(emptyList())

    /** [workId]'s download when it's ready to play here; null otherwise. */
    fun readyDownload(workId: String): DownloadItem? = downloads.value.firstOrNull { it.workId == workId && it.status == DownloadStatus.Ready }

    /** Asks the server what it is again, as Try again does on a server the app doesn't work with. */
    fun checkCompatibility() {
        scope.launch {
            try {
                session.serverInfo()
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // It stays as it last said it is.
            }
        }
    }

    private val bookPlayback = lazy { BookPlayback(this) }

    /** The phone's book player, made as it's first wanted. */
    val books: BookPlayback by bookPlayback

    /** Ends the app's session on the server and forgets the token. */
    fun signOut() {
        graph.scope.launch { session.signOut() }
    }

    /** Signs out as [signOut] does, leaving for the sign-in screens to choose a server rather than for this one's sign-in code. */
    fun signOutAndChangeServer() {
        graph.changeServer()
        signOut()
    }

    fun close() {
        if (bookPlayback.isInitialized()) books.close()
        scope.cancel()
        images.shutdown()
        session.close()
    }
}
