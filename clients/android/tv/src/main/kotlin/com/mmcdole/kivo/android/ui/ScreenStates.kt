package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.words
import com.mmcdole.kivo.core.Compatibility
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.display.problem
import kotlinx.coroutines.delay

// Every state a screen shows has a title, a sentence and one action.

/** A labelled spinner: no spinner without a label. */
@Composable
fun Loading(modifier: Modifier = Modifier) {
    Box(modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Text(stringResource(R.string.loading), style = TvType.body, color = Kivo.colors.textMuted)
    }
}

/** An empty or failed state: a title, a sentence and an action. */
@Composable
fun EmptyState(
    title: String,
    sentence: String?,
    modifier: Modifier = Modifier,
    actions: @Composable () -> Unit = {},
) {
    Box(modifier.fillMaxSize().padding(Kivo.space(12)), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
            Text(title, style = TvType.heading, color = Kivo.colors.text, textAlign = TextAlign.Center)
            if (sentence != null) {
                Text(sentence, style = TvType.body, color = Kivo.colors.textMuted, textAlign = TextAlign.Center, modifier = Modifier.widthIn(max = 520.dp))
            }
            Row(horizontalArrangement = Arrangement.spacedBy(Kivo.space(3))) { actions() }
        }
    }
}

/**
 * A read that failed. Without an answer from the server it's the full-screen
 * "Can't reach {server}. Retrying…", counting down to the next try, with
 * Retry now and Change server; a problem says what it was, with
 * Try again.
 */
@Composable
fun Failed(problem: RequestProblem, server: String, onRetry: () -> Unit, onChangeServer: () -> Unit) {
    val retry = remember { FocusRequester() }
    if (problem is RequestProblem.Unreachable) {
        EmptyState(
            stringResource(R.string.unreachable_title, server),
            stringResource(R.string.unreachable_retrying, retryCountdown(onRetry)),
        ) {
            KivoButton(stringResource(R.string.retry_now), onRetry, Modifier.focusRequester(retry), primary = true)
            KivoButton(stringResource(R.string.change_server), onChangeServer)
        }
    } else {
        EmptyState(stringResource(R.string.couldnt_load), display.problem(problem)) {
            KivoButton(stringResource(R.string.try_again), onRetry, Modifier.focusRequester(retry), primary = true)
        }
    }
    LaunchedEffect(problem) { focusWhenPlaced(retry) }
}

/**
 * A server the app doesn't work with, in place of its sign-in or its
 * screens: which of the two needs an update, with Try again, focused,
 * which asks the server what it is again, and Change server.
 */
@Composable
fun Incompatible(problem: Compatibility, server: String, onRetry: () -> Unit, onChangeServer: () -> Unit) {
    val words = problem.words(LocalResources.current, server) ?: return
    val retry = remember { FocusRequester() }
    EmptyState(words.title, words.sentence, Modifier.testTag("incompatible")) {
        KivoButton(stringResource(R.string.try_again), onRetry, Modifier.focusRequester(retry), primary = true)
        KivoButton(stringResource(R.string.change_server), onChangeServer)
    }
    FocusOnOpen(retry, problem)
}

/**
 * A read that failed under what the screen already shows, such as a
 * detail page's hero or one of its rows: what [Failed] says, where what's
 * missing would be, with its one action, which takes focus when [focus],
 * on a screen with nothing else to focus; else focus stays where it is.
 */
@Composable
fun InlineFailed(problem: RequestProblem, server: String, onRetry: () -> Unit, modifier: Modifier = Modifier, focus: Boolean = false) {
    val retry = remember { FocusRequester() }
    val unreachable = problem is RequestProblem.Unreachable
    Column(modifier, verticalArrangement = Arrangement.spacedBy(Kivo.space(2))) {
        Text(
            if (unreachable) stringResource(R.string.unreachable_title, server) else stringResource(R.string.couldnt_load),
            style = TvType.bodyStrong,
            color = Kivo.colors.text,
        )
        Text(
            if (unreachable) stringResource(R.string.unreachable_retrying, retryCountdown(onRetry)) else display.problem(problem),
            style = TvType.small,
            color = Kivo.colors.textMuted,
        )
        Box(Modifier.padding(top = Kivo.space(1))) {
            KivoButton(stringResource(if (unreachable) R.string.retry_now else R.string.try_again), onRetry, Modifier.focusRequester(retry).testTag("retryLoad"))
        }
    }
    if (focus) LaunchedEffect(problem) { focusWhenPlaced(retry) }
}

/** The seconds until a read with no answer is asked again, counting down to [onRetry]. */
@Composable
private fun retryCountdown(onRetry: () -> Unit): Int {
    var seconds by remember { mutableIntStateOf(RETRY_SECONDS) }
    LaunchedEffect(Unit) {
        while (seconds > 0) {
            delay(1_000)
            seconds--
        }
        onRetry()
    }
    return seconds
}

private const val RETRY_SECONDS = 10
