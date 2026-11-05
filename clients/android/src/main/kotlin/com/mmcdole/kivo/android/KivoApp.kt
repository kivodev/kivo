package com.mmcdole.kivo.android

import android.app.Application
import com.mmcdole.kivo.android.watchnext.WatchNext

/**
 * The process: it holds the [AppGraph], opens the last server at start,
 * tells the core as the network changes and keeps the Watch Next row up to
 * date.
 */
class KivoApp : Application() {
    lateinit var graph: AppGraph
        private set

    override fun onCreate() {
        super.onCreate()
        graph = AppGraph(this)
        graph.start()
        watchNetwork(this, graph.core)
        WatchNext.schedule(this)
    }
}
