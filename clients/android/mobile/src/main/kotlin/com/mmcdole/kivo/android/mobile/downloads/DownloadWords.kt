package com.mmcdole.kivo.android.mobile.downloads

import androidx.compose.runtime.Composable
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.core.display.runtime
import com.mmcdole.kivo.core.display.size
import com.mmcdole.kivo.core.downloads.DownloadItem
import com.mmcdole.kivo.core.downloads.DownloadStatus
import com.mmcdole.kivo.core.downloads.WaitReason
import com.mmcdole.kivo.core.downloads.downloadFailure
import kotlin.math.floor

/** Where a download under way is: in line, prepared on the server, coming, waiting for something, or paused. */
enum class Stage { Queued, Preparing, Downloading, WaitingForWifi, WaitingForServer, Offline, Paused }

/**
 * Where a download under way is, as its line says it and its ring draws it:
 * a stage, and how much of the whole job is done, 0 to 100, once there's
 * something to measure. In line with nothing done, there's no percent.
 */
data class DownloadProgress(val stage: Stage, val percent: Int?) {
    /** The ring's fill, 0 to 1. */
    val fraction: Float? get() = percent?.let { it / 100f }

    /** The accent while it's prepared or comes; grey while it's in line, waits or is paused. */
    val active: Boolean get() = stage == Stage.Preparing || stage == Stage.Downloading

    companion object {
        fun of(stage: Stage, percent: Int?) = DownloadProgress(stage, percent.takeUnless { stage == Stage.Queued && it == 0 })
    }
}

/** Downloads' state as their rows, rings and a detail page's button show it. */
object DownloadWords {
    /**
     * How much of a download's whole job is done, 0 to 100, rounded down so
     * it's 100 only once it's here. One the server converts is half done once
     * converted and the rest as it comes; an original only comes. Null while
     * nothing has started, while the server prepares only an original's
     * subtitle, and once it failed.
     */
    fun done(item: DownloadItem): Int? {
        val share = when (val s = item.status) {
            DownloadStatus.Ready -> return 100
            is DownloadStatus.Queued, is DownloadStatus.Failed -> return null
            is DownloadStatus.Converting -> if (item.prepared) s.progress / 2 else return null
            is DownloadStatus.Downloading -> came(item, s.receivedBytes, s.totalBytes)
            is DownloadStatus.Waiting -> came(item, s.receivedBytes, s.totalBytes)
            is DownloadStatus.Paused -> came(item, s.receivedBytes, s.totalBytes)
        }
        return floor(share.coerceIn(0.0, 1.0) * 100 + 1e-9).toInt().coerceAtMost(99)
    }

    private fun came(item: DownloadItem, received: Long, total: Long): Double {
        val here = if (total > 0) received.toDouble() / total else 0.0
        return if (item.prepared) 0.5 + here / 2 else here
    }

    /** Where a download under way is; null once it's here or failed. */
    fun progress(item: DownloadItem): DownloadProgress? {
        val done = done(item)
        return when (val s = item.status) {
            is DownloadStatus.Queued -> DownloadProgress.of(Stage.Queued, null)
            is DownloadStatus.Converting -> DownloadProgress.of(Stage.Preparing, done)
            is DownloadStatus.Downloading -> DownloadProgress.of(Stage.Downloading, done)
            is DownloadStatus.Waiting -> DownloadProgress.of(
                when (s.reason) {
                    WaitReason.Turn -> Stage.Queued
                    WaitReason.Wifi -> Stage.WaitingForWifi
                    WaitReason.Server -> Stage.WaitingForServer
                    WaitReason.Network -> Stage.Offline
                },
                done,
            )
            is DownloadStatus.Paused -> DownloadProgress.of(Stage.Paused, done)
            DownloadStatus.Ready, is DownloadStatus.Failed -> null
        }
    }

    /**
     * Where a show's episodes are together, each that hasn't failed counting
     * the same: Downloading while any is prepared or comes, else what holds
     * them, a wait, a pause or the line; null once every one of them is here.
     */
    fun progress(episodes: List<DownloadItem>): DownloadProgress? {
        val counted = episodes.filter { it.status !is DownloadStatus.Failed }
        val stages = counted.mapNotNull { progress(it)?.stage }
        val stage = when {
            stages.any { it == Stage.Preparing || it == Stage.Downloading } -> Stage.Downloading
            else -> stages.firstOrNull { it == Stage.WaitingForWifi || it == Stage.WaitingForServer || it == Stage.Offline }
                ?: listOf(Stage.Paused, Stage.Queued).firstOrNull { it in stages }
        } ?: return null
        return DownloadProgress.of(stage, counted.sumOf { done(it) ?: 0 } / counted.size)
    }
}

/** "Downloading · 52%", or "Queued" with no percent. */
@Composable
fun words(progress: DownloadProgress): String {
    val word = stringResource(
        when (progress.stage) {
            Stage.Queued -> R.string.mobile_download_queued
            Stage.Preparing -> R.string.mobile_download_preparing
            Stage.Downloading -> R.string.mobile_download_downloading
            Stage.WaitingForWifi -> R.string.mobile_download_waiting_wifi
            Stage.WaitingForServer -> R.string.mobile_download_waiting_server
            Stage.Offline -> R.string.mobile_download_offline
            Stage.Paused -> R.string.mobile_download_paused
        },
    )
    return progress.percent?.let { stringResource(R.string.mobile_download_percent, word, it) } ?: word
}

/**
 * A row's line under its title. While it's under way, its state and how
 * much of the whole job is done; once it's here, how long it plays and its
 * size, a book's authors in its runtime's place. A failed one says only why.
 */
@Composable
fun downloadLine(item: DownloadItem): String {
    DownloadWords.progress(item)?.let { return words(it) }
    (item.status as? DownloadStatus.Failed)?.let { return display.downloadFailure(it.reason) }
    val what = if (item.kind == "book") item.authors else item.durationMs?.takeIf { it > 0 }?.let { display.runtime(it) }
    return listOfNotNull(what, display.size(item.sizeBytes)).joinToString(" · ")
}

/**
 * A show's line under its name: "6 episodes · 2.3 GB" once every episode
 * that hasn't failed is here, else one state for the whole show. When every
 * one failed, why the first did.
 */
@Composable
fun showLine(episodes: List<DownloadItem>): String {
    DownloadWords.progress(episodes)?.let { return words(it) }
    val ready = episodes.filter { it.status == DownloadStatus.Ready }
    if (ready.isEmpty()) episodes.firstNotNullOfOrNull { it.status as? DownloadStatus.Failed }?.let { return display.downloadFailure(it.reason) }
    return pluralStringResource(R.plurals.mobile_download_episodes_size, ready.size, ready.size, display.size(ready.sumOf { it.sizeBytes }))
}

/** A row's title: an episode's number leads it on its show's page ([numbered]), "2. Trouble in Lumpy Space". */
fun downloadTitle(item: DownloadItem, numbered: Boolean): String =
    item.episode?.takeIf { numbered }?.let { "$it. ${item.title}" } ?: item.title

/** The state in short, under a detail page's actions: "Downloaded" once it's here, "Download failed", else the line's words. */
@Composable
fun downloadBadge(item: DownloadItem): String {
    DownloadWords.progress(item)?.let { return words(it) }
    return stringResource(if (item.status is DownloadStatus.Failed) R.string.mobile_download_failed else R.string.mobile_downloaded)
}
