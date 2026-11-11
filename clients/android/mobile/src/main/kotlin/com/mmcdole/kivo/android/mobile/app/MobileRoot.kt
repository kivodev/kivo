package com.mmcdole.kivo.android.mobile.app

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTagsAsResourceId
import androidx.lifecycle.ViewModelStore
import androidx.lifecycle.ViewModelStoreOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.LocalViewModelStoreOwner
import com.mmcdole.kivo.android.AppGraph
import com.mmcdole.kivo.core.session.AppState
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.signin.SignInScreen
import com.mmcdole.kivo.android.mobile.theme.KivoMobileTheme
import com.mmcdole.kivo.android.mobile.ui.Loading
import com.mmcdole.kivo.android.mobile.ui.LocalSession
import com.mmcdole.kivo.android.mobile.ui.Message

/** The phone and tablet interface: sign-in until the app has a server, then its screens. */
@Composable
fun MobileRoot(graph: AppGraph) {
    KivoMobileTheme {
        // Test tags double as resource IDs, so UI Automator finds controls on a device.
        Surface(Modifier.fillMaxSize().semantics { testTagsAsResourceId = true }, color = MaterialTheme.colorScheme.background) {
            val state by graph.core.state.collectAsStateWithLifecycle()
            val session by graph.session.collectAsStateWithLifecycle()
            when (val s = state) {
                AppState.Starting -> Loading()
                is AppState.SignedOut -> ViewModels(s) { SignInScreen(graph, s.server, s.identityChanged) }
                is AppState.Unreachable -> Message(stringResource(R.string.mobile_cant_reach, s.server.name), stringResource(R.string.mobile_cant_reach_saved_sentence)) {
                    Button(onClick = graph.core::retry) { Text(stringResource(R.string.mobile_retry)) }
                    OutlinedButton(onClick = graph::changeServer) { Text(stringResource(R.string.mobile_change_server)) }
                }
                is AppState.SignedIn -> when (val signedIn = session?.takeIf { it.session === s.session }) {
                    null -> Loading()
                    else -> ViewModels(signedIn) {
                        CompositionLocalProvider(LocalSession provides signedIn) { MainScaffold(signedIn) }
                    }
                }
            }
        }
    }
}

/**
 * View models of their own for [content], while [key] lasts: a sign-in or
 * a signed-in server's screens start afresh, and theirs are cleared as they go.
 */
@Composable
private fun ViewModels(key: Any, content: @Composable () -> Unit) {
    val owner = remember(key) {
        object : ViewModelStoreOwner {
            override val viewModelStore = ViewModelStore()
        }
    }
    DisposableEffect(owner) { onDispose { owner.viewModelStore.clear() } }
    CompositionLocalProvider(LocalViewModelStoreOwner provides owner, content = content)
}
