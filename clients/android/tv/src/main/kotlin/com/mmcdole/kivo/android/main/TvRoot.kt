package com.mmcdole.kivo.android.main

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTagsAsResourceId
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mmcdole.kivo.android.AppGraph
import com.mmcdole.kivo.core.session.AppState
import com.mmcdole.kivo.android.signin.SignInScreen
import com.mmcdole.kivo.android.signin.UnreachableScreen
import com.mmcdole.kivo.android.theme.KivoTheme
import com.mmcdole.kivo.android.ui.Incompatible
import com.mmcdole.kivo.android.ui.Loading
import com.mmcdole.kivo.android.ui.LocalSession
import com.mmcdole.kivo.core.Compatibility

/**
 * The TV interface: sign-in until the app has a server, then its screens,
 * or which of the app and the server needs an update while they don't work
 * together; [onExit] leaves the app.
 */
@Composable
fun TvRoot(graph: AppGraph, onExit: () -> Unit) {
    KivoTheme {
        val state by graph.core.state.collectAsStateWithLifecycle()
        val session by graph.session.collectAsStateWithLifecycle()
        // Test tags double as resource IDs, so UI Automator finds controls on a device.
        // The window's background is the page's, so nothing here paints the whole screen again.
        Box(Modifier.fillMaxSize().semantics { testTagsAsResourceId = true }) {
            when (val s = state) {
                AppState.Starting -> Loading()
                is AppState.SignedOut -> SignInScreen(graph, s.server, s.identityChanged)
                is AppState.Unreachable -> UnreachableScreen(graph, s.server)
                is AppState.SignedIn -> when (val signedIn = session?.takeIf { it.session === s.session }) {
                    null -> Loading()
                    else -> {
                        val compatibility by s.session.compatibility.collectAsStateWithLifecycle()
                        if (compatibility == Compatibility.Compatible) {
                            CompositionLocalProvider(LocalSession provides signedIn) { MainScreen(signedIn, onExit = onExit) }
                        } else {
                            val server by s.session.savedServer.collectAsStateWithLifecycle()
                            Incompatible(compatibility, server.name, signedIn::checkCompatibility, graph::changeServer)
                        }
                    }
                }
            }
        }
    }
}
