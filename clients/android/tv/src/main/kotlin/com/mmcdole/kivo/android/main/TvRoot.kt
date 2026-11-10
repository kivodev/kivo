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
import com.mmcdole.kivo.android.AppState
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
        val state by graph.state.collectAsStateWithLifecycle()
        // Test tags double as resource IDs, so UI Automator finds controls on a device.
        // The window's background is the page's, so nothing here paints the whole screen again.
        Box(Modifier.fillMaxSize().semantics { testTagsAsResourceId = true }) {
            when (val s = state) {
                AppState.Starting -> Loading()
                is AppState.SigningIn -> SignInScreen(graph, s.server)
                is AppState.Unreachable -> UnreachableScreen(graph, s.server)
                is AppState.SignedIn -> {
                    val compatibility by s.session.session.compatibility.collectAsStateWithLifecycle()
                    if (compatibility == Compatibility.Compatible) {
                        CompositionLocalProvider(LocalSession provides s.session) { MainScreen(s.session, onExit = onExit) }
                    } else {
                        val server by s.session.session.savedServer.collectAsStateWithLifecycle()
                        Incompatible(compatibility, server.name, s.session::checkCompatibility, graph::changeServer)
                    }
                }
            }
        }
    }
}
