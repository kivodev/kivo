package com.mmcdole.kivo.android.downloads

import android.content.Context
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.mmcdole.kivo.android.AppState
import com.mmcdole.kivo.android.HasAppGraph
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withTimeoutOrNull
import java.util.concurrent.TimeUnit
import kotlin.time.Duration.Companion.minutes

/**
 * The downloads' refresh while the app isn't open: a quarter of an hour
 * after it leaves the foreground with a download the server is still
 * preparing, the app opens its server and the core starts what the server
 * has ready on the transfers; it's asked for again while any is still
 * being prepared.
 */
class DownloadRefresh(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val graph = (applicationContext as HasAppGraph).graph
        val signedIn = withTimeoutOrNull(OPEN_WAIT) { graph.state.first { it !is AppState.Starting } } as? AppState.SignedIn ?: return Result.success()
        val pending = try {
            signedIn.session.session.downloads?.startReady() == true
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            true
        }
        if (pending) schedule(applicationContext)
        return Result.success()
    }

    companion object {
        private const val NAME = "download-refresh"
        private val OPEN_WAIT = 1.minutes

        /** Asks for a refresh in a quarter of an hour, on any network, in place of one asked for before. */
        fun schedule(context: Context) {
            WorkManager.getInstance(context).enqueueUniqueWork(
                NAME,
                ExistingWorkPolicy.REPLACE,
                OneTimeWorkRequestBuilder<DownloadRefresh>()
                    .setInitialDelay(15, TimeUnit.MINUTES)
                    .setConstraints(Constraints(requiredNetworkType = NetworkType.CONNECTED))
                    .build(),
            )
        }
    }
}
