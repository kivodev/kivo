package com.mmcdole.kivo.android

import android.app.Application
import com.mmcdole.kivo.android.watchnext.WatchNext
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

/**
 * The process: it holds the [AppGraph], opens the last server at start,
 * tells the core as the network changes, starts the playback service as
 * the phone's player offers itself to it and, on a TV, keeps the Watch Next
 * row up to date.
 */
class KivoApp : Application(), HasAppGraph {
    override lateinit var graph: AppGraph
        private set

    override fun onCreate() {
        super.onCreate()
        val tv = chooseUi(this) == Ui.Tv
        graph = AppGraph(
            this,
            AppBuild(BuildConfig.VERSION_NAME, R.raw.aboutlibraries),
            onOpened = { if (tv) WatchNext.refresh(this) },
            onLeft = { if (tv) WatchNext.clear(this) },
        )
        graph.start()
        graph.scope.launch {
            graph.sessionPlayers.current.map { it != null }.distinctUntilChanged().filter { it }.collect { PlaybackService.start(this@KivoApp) }
        }
        watchNetwork(this, graph.core)
        if (tv) WatchNext.schedule(this)
    }
}
