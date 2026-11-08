package com.mmcdole.kivo.android.mobile.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.core.RequestProblem

/** A spinner in the middle of the screen. */
@Composable
fun Loading(modifier: Modifier = Modifier) {
    Box(modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
}

/** A heading and a sentence in the middle of the screen, with an action under them. */
@Composable
fun Message(title: String, sentence: String?, modifier: Modifier = Modifier, action: (@Composable () -> Unit)? = null) {
    Box(modifier.fillMaxSize().padding(space(6)), contentAlignment = Alignment.Center) {
        Column(Modifier.widthIn(max = 420.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(space(2))) {
            Text(title, style = MaterialTheme.typography.titleMedium, textAlign = TextAlign.Center)
            if (sentence != null) {
                Text(sentence, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
            }
            if (action != null) Box(Modifier.padding(top = space(2))) { action() }
        }
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
