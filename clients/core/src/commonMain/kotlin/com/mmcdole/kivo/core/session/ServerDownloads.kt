package com.mmcdole.kivo.core.session

import co.touchlab.skie.configuration.annotations.DefaultArgumentInterop
import com.mmcdole.kivo.api.models.DownloadEstimate
import com.mmcdole.kivo.api.models.DownloadEstimateItem
import com.mmcdole.kivo.core.downloads.DownloadItem
import com.mmcdole.kivo.core.downloads.DownloadRefusedException
import com.mmcdole.kivo.core.downloads.DownloadRequest
import com.mmcdole.kivo.core.downloads.DownloadStorage
import com.mmcdole.kivo.core.downloads.Downloads
import com.mmcdole.kivo.core.downloads.NotEnoughSpaceException
import com.mmcdole.kivo.core.play.PlayChoice
import com.mmcdole.kivo.core.settings.DownloadQuality
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.catch

/** A server's downloads on the device ([KivoSession.downloads]): the list, and what the user does with them. */
class SessionDownloads internal constructor(private val downloads: Downloads) {
    /** This server's downloads on the device, newest first. */
    val items: Flow<List<DownloadItem>> = downloads.items.catch { emit(emptyList()) }

    /** The works being asked of the server now, before their downloads are listed: a download button shows it's waiting. */
    val asking: Flow<Set<String>> = downloads.asking.catch { emit(emptySet()) }

    /** How many downloads the availability checks deleted, until [deletionsSeen]. */
    val deleted: Flow<Int> = downloads.deleted.catch { emit(0) }

    /**
     * What downloading each of [choices] would be at each quality, for the
     * download menu; a season's or series' episodes in one call.
     */
    @Throws(Exception::class)
    suspend fun estimates(choices: List<PlayChoice>): List<DownloadEstimate> = downloads.estimate(
        choices.map { DownloadEstimateItem(workId = it.workId, versionId = it.versionId, audioTrackId = it.audioTrackId, subtitleTrackId = it.subtitleTrackId) },
    )

    /**
     * Downloads [choice] at [quality], or the download quality setting's when
     * null; throws [NotEnoughSpaceException] or [DownloadRefusedException].
     * Returns the download's ID.
     */
    @DefaultArgumentInterop.Enabled
    @Throws(Exception::class)
    suspend fun download(choice: PlayChoice, quality: DownloadQuality? = null): String =
        downloads.download(DownloadRequest(choice.workId, choice.versionId, choice.audioTrackId, choice.subtitleTrackId, quality))

    @Throws(Exception::class)
    suspend fun delete(id: String) = downloads.delete(id)

    /** Deletes a show's or a selection's downloads, cancelling those still under way. */
    @Throws(Exception::class)
    suspend fun delete(ids: List<String>) = downloads.delete(ids)

    @Throws(Exception::class)
    suspend fun retry(id: String) = downloads.retry(id)

    /** Pauses a download coming to the device, keeping what arrived. */
    @Throws(Exception::class)
    suspend fun pause(id: String) = downloads.pause(id)

    /** Resumes a paused download from where it stopped. */
    @Throws(Exception::class)
    suspend fun resume(id: String) = downloads.resume(id)

    /** Brings the downloads up to date with the server: on returning to the foreground, and once the server is reached. */
    @Throws(Exception::class)
    suspend fun sync() = downloads.sync()

    /**
     * Starts the downloads the server has finished preparing, for the
     * platform's background refresh. Returns whether any is still in line
     * or being prepared, so the refresh is asked for again.
     */
    @Throws(Exception::class)
    suspend fun startReady(): Boolean = downloads.startReady()

    /** The space downloads take, and what's free beside them. */
    @Throws(Exception::class)
    suspend fun storage(): DownloadStorage = downloads.storage()

    /** The Downloads list has said how many downloads the checks deleted. */
    fun deletionsSeen() = downloads.deletionsSeen()
}
