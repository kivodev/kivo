package com.mmcdole.kivo.android

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTagsAsResourceId
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import com.mmcdole.kivo.android.main.MainScreen
import com.mmcdole.kivo.android.signin.SignInScreen
import com.mmcdole.kivo.android.signin.UnreachableScreen
import com.mmcdole.kivo.android.theme.KivoTheme
import com.mmcdole.kivo.android.ui.Loading
import com.mmcdole.kivo.android.ui.LocalSession
import com.mmcdole.kivo.android.watchnext.WatchNext
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.launch

/**
 * The one activity. Live events run only while it's in the foreground:
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

        setContent {
            KivoTheme {
                val state by graph.state.collectAsStateWithLifecycle()
                // Test tags double as resource IDs, so UI Automator finds controls on a device.
                // The window's background is the page's, so nothing here paints the whole screen again.
                Box(Modifier.fillMaxSize().semantics { testTagsAsResourceId = true }) {
                    when (val s = state) {
                        AppState.Starting -> Loading()
                        is AppState.SigningIn -> SignInScreen(graph, s.server)
                        is AppState.Unreachable -> UnreachableScreen(graph, s.server)
                        is AppState.SignedIn -> CompositionLocalProvider(LocalSession provides s.session) {
                            MainScreen(s.session, onExit = ::finish)
                        }
                    }
                }
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
