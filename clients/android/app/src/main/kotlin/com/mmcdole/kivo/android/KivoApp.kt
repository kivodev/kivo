package com.mmcdole.kivo.android

import android.app.Application
import com.mmcdole.kivo.android.watchnext.WatchNext

/**
 * The process: it holds the [AppGraph], opens the last server at start,
 * tells the core as the network changes and, on a TV, keeps the Watch Next
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
        watchNetwork(this, graph.core)
        if (tv) WatchNext.schedule(this)
    }
}
