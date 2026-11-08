package com.mmcdole.kivo.android.downloads

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.pm.ServiceInfo
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.work.CoroutineWorker
import androidx.work.ForegroundInfo
import androidx.work.WorkerParameters
import com.mmcdole.kivo.android.HasAppGraph
import com.mmcdole.kivo.android.platform.R
import com.mmcdole.kivo.core.downloads.TransferEvent
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.sync.withLock
import java.io.File

/**
 * One file of a download coming to the phone, for [WorkTransport]: fetched
 * by [RangeFetch], telling the core how it goes. A file that takes more than
 * a moment runs in the foreground, its notification showing the download's
 * title and how much of the file is here.
 */
class DownloadWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val transport = (applicationContext as HasAppGraph).graph.transport ?: return Result.failure()
        val id = inputData.getString(ID) ?: return Result.failure()
        val request = transport.request(id)
        if (request == null) {
            // The process started afresh: the core starts it again with its token.
            transport.events(TransferEvent.Failed(id, 0, "not started in this process"))
            return Result.failure()
        }
        val title = transport.titleOf(id.substringBefore('/'))
        val began = System.currentTimeMillis()
        var foreground = false
        return transport.lock(id).withLock {
            val end = RangeFetch(request.url, request.authorization, File(request.destination)) { received, expected ->
                transport.events(TransferEvent.Progress(id, received, expected))
                if (!foreground && System.currentTimeMillis() - began >= FOREGROUND_AFTER_MS) foreground = foreground(title, 0)
                if (foreground) notify(title, percent(received, expected))
            }.run()
            when (end) {
                FetchEnd.Whole -> {
                    transport.events(TransferEvent.Finished(id))
                    Result.success()
                }
                is FetchEnd.Stopped -> {
                    transport.events(TransferEvent.Failed(id, end.status, end.message))
                    Result.failure()
                }
            }
        }
    }

    /** Runs in the foreground with the download's notification; false where the system doesn't allow it now, as from the background. */
    private suspend fun foreground(title: String?, percent: Int): Boolean = try {
        setForeground(foregroundInfo(title, percent))
        true
    } catch (e: CancellationException) {
        throw e
    } catch (_: Exception) {
        false
    }

    private fun notify(title: String?, percent: Int) {
        applicationContext.getSystemService(NotificationManager::class.java)?.notify(NOTIFICATION, notification(title, percent))
    }

    override suspend fun getForegroundInfo(): ForegroundInfo = foregroundInfo(null, 0)

    private fun foregroundInfo(title: String?, percent: Int): ForegroundInfo {
        val notification = notification(title, percent)
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            ForegroundInfo(NOTIFICATION, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        } else {
            ForegroundInfo(NOTIFICATION, notification)
        }
    }

    private fun notification(title: String?, percent: Int): android.app.Notification {
        val context = applicationContext
        val manager = context.getSystemService(NotificationManager::class.java)
        if (manager?.getNotificationChannel(CHANNEL) == null) {
            manager?.createNotificationChannel(NotificationChannel(CHANNEL, context.getString(R.string.downloads_channel), NotificationManager.IMPORTANCE_LOW))
        }
        val open = context.packageManager.getLaunchIntentForPackage(context.packageName)?.let {
            PendingIntent.getActivity(context, 0, it, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        }
        return NotificationCompat.Builder(context, CHANNEL)
            .setSmallIcon(android.R.drawable.stat_sys_download)
            .setContentTitle(title ?: context.getString(R.string.downloading))
            .setContentText(context.getString(R.string.downloading_percent, percent))
            .setProgress(100, percent, false)
            .setContentIntent(open)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setSilent(true)
            .build()
    }

    companion object {
        const val ID = "id"
        private const val CHANNEL = "downloads"
        private const val NOTIFICATION = 0x6b69
        private const val FOREGROUND_AFTER_MS = 1_000L

        /** How much of a file is here, 0 to 100. */
        internal fun percent(received: Long, expected: Long): Int =
            if (expected <= 0) 0 else (received * 100 / expected).toInt().coerceIn(0, 100)
    }
}
