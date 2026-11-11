package com.mmcdole.kivo.core.downloads

import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.display.fill
import com.mmcdole.kivo.core.display.problem
import com.mmcdole.kivo.core.display.bitrate
import com.mmcdole.kivo.core.display.size
import com.mmcdole.kivo.core.settings.DownloadQuality
import kotlinx.coroutines.CancellationException

/** A download as the Downloads list and a detail page show it. */
data class DownloadItem(
    val id: String,
    val workId: String,
    val versionId: String,
    val title: String,
    val kind: WorkKind,
    /** An episode's series and its place in it. */
    val seriesId: String?,
    val seriesTitle: String?,
    val season: Int?,
    val episode: Int?,
    /** A book's authors, "Frank Herbert, Brian Herbert". */
    val authors: String?,
    /** Null for one this version doesn't know. */
    val quality: DownloadQuality?,
    /** How long it plays, as the server's plan of it says; null when the plan isn't readable. */
    val durationMs: Long?,
    /** Its files' size: exact when [sizeExact], else the most the server's conversion can come to. */
    val sizeBytes: Long,
    /** [sizeBytes] is exact: the server has its files ready, or sends the original as it is. */
    val sizeExact: Boolean,
    /** The server converts it before it comes, as its plan says from the start; false when the original comes as it is. */
    val prepared: Boolean,
    val status: DownloadStatus,
    /** Its artwork on the device, once fetched: an episode's still, else its poster. */
    val artworkPath: String?,
    /** An episode's series poster on the device, once fetched; null for anything else. */
    val posterPath: String?,
    val createdAtMs: Long,
)

/** Where a download is. */
sealed interface DownloadStatus {
    /** In line on the server. */
    data object Queued : DownloadStatus

    /**
     * The server is preparing it: converting it, from 0 to 1 with about
     * [remainingMs] left once that's known, or, when the item isn't
     * [DownloadItem.prepared], preparing its subtitle.
     */
    data class Converting(val progress: Double, val remainingMs: Long?) : DownloadStatus

    /** Its files are on their way: [receivedBytes] of [totalBytes], at [bytesPerSecond]. */
    data class Downloading(val receivedBytes: Long, val totalBytes: Long, val bytesPerSecond: Long) : DownloadStatus

    /** Its files would be on their way, with [receivedBytes] of [totalBytes] here, but wait for [reason]. */
    data class Waiting(val reason: WaitReason, val receivedBytes: Long, val totalBytes: Long) : DownloadStatus

    /** The user paused it with [receivedBytes] of [totalBytes] here; resuming goes on from there. */
    data class Paused(val receivedBytes: Long, val totalBytes: Long) : DownloadStatus

    /** Every file is here and checked: it plays with no server. */
    data object Ready : DownloadStatus

    /**
     * It stopped, for [reason]: a playback reason code from the server, or
     * the device's own, [NOT_ENOUGH_SPACE], [CHECK_FAILED] or [NOT_PLAYABLE].
     * Retry asks again.
     */
    data class Failed(val reason: String) : DownloadStatus

    companion object {
        /** The device hasn't the room for it. */
        const val NOT_ENOUGH_SPACE = "notEnoughSpace"

        /** A file kept arriving other than the server said it would. */
        const val CHECK_FAILED = "checkFailed"

        /** The server can't make it for this app. */
        const val NOT_PLAYABLE = "notPlayable"
    }
}

/** What a download's transfers wait for. */
enum class WaitReason {
    /** The device has no network. */
    Network,

    /** The device is on cellular, or another expensive network, and cellular downloads aren't allowed. */
    Wifi,

    /** Other downloads come first: only so many come at once, in the order they were asked for. */
    Turn,

    /** A transfer couldn't reach the server, and starts again after a wait, or once the server answers. */
    Server,
}

/** A download quality's name: "Original", "High" or "Standard". */
fun Display.downloadQuality(quality: DownloadQuality): String = table.downloads.qualities[quality.value] ?: quality.value

/** Why a download failed ([DownloadStatus.Failed]): the device's own reason, or the server's reason code. */
fun Display.downloadFailure(reason: String): String = table.downloads.failures[reason] ?: reason(reason)

/**
 * What came of asking for a download, or for a failed one again: it's on
 * its way ([Started]), or why not: too little room on the device
 * ([NoRoom]), the server can't make it for this app ([Refused], with its
 * reason code), or the request failed ([Failed]).
 */
sealed class DownloadStart {
    data class Started(val id: String) : DownloadStart()

    data class NoRoom(val shortBytes: Long) : DownloadStart()

    data class Refused(val reason: String) : DownloadStart()

    data class Failed(val problem: RequestProblem) : DownloadStart()
}

/** [ask]'s download as [DownloadStart]; cancellation goes on to the caller. */
internal suspend fun started(ask: suspend () -> String): DownloadStart = try {
    DownloadStart.Started(ask())
} catch (e: CancellationException) {
    throw e
} catch (e: NotEnoughSpaceException) {
    DownloadStart.NoRoom(e.shortBytes)
} catch (e: DownloadRefusedException) {
    DownloadStart.Refused(e.reason)
} catch (e: Exception) {
    DownloadStart.Failed(RequestProblem.of(e))
}

/**
 * Why a download couldn't start, in a sentence: too little room, with how
 * much to free; the server refusing it; or the request's problem. Null when
 * it started.
 */
fun Display.downloadProblem(start: DownloadStart): String? = when (start) {
    is DownloadStart.Started -> null
    is DownloadStart.NoRoom -> table.downloads.notEnoughSpace.fill("size" to size(start.shortBytes))
    is DownloadStart.Refused -> if (start.reason == "downloadsNotAllowed") reason(start.reason) else table.downloads.refused
    is DownloadStart.Failed -> problem(start.problem)
}

/** The device's network as the platform sees it: whether it has one, and whether it's cellular or another expensive one. */
internal data class NetworkPath(val online: Boolean, val expensive: Boolean)
