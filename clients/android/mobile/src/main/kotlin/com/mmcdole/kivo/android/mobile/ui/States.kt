package com.mmcdole.kivo.android.mobile.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.Button
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.mmcdole.kivo.android.STORE_LISTING
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.platform.R as PlatformR
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.words
import com.mmcdole.kivo.core.Compatibility
import com.mmcdole.kivo.core.RequestProblem

/** A spinner in the middle of the screen. */
@Composable
fun Loading(modifier: Modifier = Modifier) {
    Box(modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
}

/** A heading and a sentence in the middle of the screen, with actions centred under them. */
@Composable
fun Message(title: String, sentence: String?, modifier: Modifier = Modifier, actions: (@Composable ColumnScope.() -> Unit)? = null) {
    Box(modifier.fillMaxSize().padding(space(6)), contentAlignment = Alignment.Center) {
        Column(Modifier.widthIn(max = 420.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(space(2))) {
            Text(title, style = MaterialTheme.typography.titleMedium, textAlign = TextAlign.Center)
            if (sentence != null) {
                Text(sentence, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
            }
            if (actions != null) {
                Column(Modifier.padding(top = space(2)), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(space(1)), content = actions)
            }
        }
    }
}

/**
 * A server the app doesn't work with, in place of its sign-in or its
 * screens: which of the two needs an update, with Try again, which asks
 * the server what it is again, and Change server.
 */
@Composable
fun Incompatible(problem: Compatibility, server: String, onRetry: () -> Unit, onChangeServer: () -> Unit, modifier: Modifier = Modifier) {
    val words = problem.words(LocalResources.current, server) ?: return
    val uris = LocalUriHandler.current
    Message(words.title, words.sentence, modifier.testTag("incompatible")) {
        if (problem is Compatibility.AppTooOld) {
            STORE_LISTING?.let { listing -> Button(onClick = { uris.openUri(listing) }) { Text(stringResource(PlatformR.string.app_too_old_store)) } }
        }
        Button(onClick = onRetry) { Text(stringResource(R.string.mobile_try_again)) }
        OutlinedButton(onClick = onChangeServer) { Text(stringResource(R.string.mobile_change_server)) }
    }
}

/**
 * Why a read failed: the server can't be reached, retrying by itself
 * meanwhile, or it refused with [title] and the core's sentence; Retry
 * either way.
 */
@Composable
fun Failed(problem: RequestProblem, title: String, server: String, onRetry: () -> Unit, modifier: Modifier = Modifier) {
    val unreachable = problem is RequestProblem.Unreachable
    Message(
        title = if (unreachable) stringResource(R.string.mobile_cant_reach, server) else title,
        sentence = if (unreachable) stringResource(R.string.mobile_retrying) else sentence(problem),
        modifier = modifier,
    ) {
        Button(onClick = onRetry) { Text(stringResource(R.string.mobile_retry)) }
    }
}
