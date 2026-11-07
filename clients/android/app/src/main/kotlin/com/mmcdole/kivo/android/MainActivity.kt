package com.mmcdole.kivo.android

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import com.mmcdole.kivo.android.main.TvRoot
import com.mmcdole.kivo.android.mobile.MobileRoot
import com.mmcdole.kivo.android.watchnext.WatchNext
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.launch

/**
 * The one activity, showing the TV interface on a TV and the phone's
 * elsewhere. Live events run only while it's in the foreground:
 * the socket closes in the background, and opening it again
 * on return refetches what's on screen.
 */
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val graph = (application as KivoApp).graph
        if (savedInstanceState == null) open(intent)

        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.STARTED) {
                graph.state.collectLatest { state -> (state as? AppState.SignedIn)?.session?.session?.runLiveEvents() }
            }
        }

        val ui = chooseUi(this)
        setContent {
            when (ui) {
                Ui.Tv -> TvRoot(graph, onExit = ::finish)
                Ui.Mobile -> MobileRoot()
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        open(intent)
    }

    /** A Watch Next program's link: its work's page, once signed in. */
    private fun open(intent: Intent?) {
        WatchNext.workOf(intent?.data)?.let { (application as KivoApp).graph.opening.value = it }
    }
}
