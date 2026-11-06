package com.mmcdole.kivo.android.ui

import android.text.format.DateFormat
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.tv.material3.DenseListItem
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.api.models.SubtitleResult
import com.mmcdole.kivo.core.data.SubtitleSearch
import com.mmcdole.kivo.core.data.SubtitleSearchProblem
import com.mmcdole.kivo.core.data.problem
import com.mmcdole.kivo.core.data.subtitleResultName
import com.mmcdole.kivo.core.display.Display
import java.util.Date
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch

/**
 * Search Subtitles, the panel Audio & Subtitles' "Search subtitles…" opens
 * in its place: the results of [search] in the server's order, each its
 * language and flags over the release it was made for. Choosing one
 * [download]s it, which throws on failure; the caller then moves on. Back
 * returns ([onDismiss]).
 */
@Composable
fun SubtitleSearchPanel(
    display: Display,
    search: suspend () -> List<SubtitleResult>,
    download: suspend (SubtitleResult) -> Unit,
    onDismiss: () -> Unit,
) {
    var results by remember { mutableStateOf<List<SubtitleResult>?>(null) }
    var problem by remember { mutableStateOf<SubtitleSearchProblem?>(null) }
    var downloading by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    val first = remember { FocusRequester() }
    val exactMatch = stringResource(R.string.exact_match)
    val colors = Kivo.colors

    LaunchedEffect(Unit) {
        try {
            results = search()
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            problem = SubtitleSearch.problem(e)
        }
    }

    fun choose(result: SubtitleResult) {
        if (downloading != null) return
        downloading = result.id
        problem = null
        scope.launch {
            try {
                download(result)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                problem = SubtitleSearch.problem(e)
            } finally {
                downloading = null
            }
        }
    }

    SidePanel(stringResource(R.string.search_subtitles_title), onDismiss) {
        val shown = results
        val message = when {
            problem != null -> display.problem(problem!!) { DateFormat.getTimeFormat(context).format(Date(it)) }
            shown == null -> stringResource(R.string.loading)
            shown.isEmpty() -> stringResource(R.string.no_subtitles_found)
            else -> null
        }
        if (message != null) {
            Text(
                message,
                style = TvType.body,
                color = colors.onPlayerMuted,
                modifier = Modifier.padding(start = Kivo.space(3), bottom = Kivo.space(3)).testTag("subtitle-search-message"),
            )
        }
        if (!shown.isNullOrEmpty()) {
            KeepInView {
                LazyColumn(Modifier.weight(1f, fill = false), contentPadding = PaddingValues(bottom = Kivo.space(6))) {
                    itemsIndexed(shown, key = { _, r -> r.id }) { i, result ->
                        val name = display.subtitleResultName(result, exactMatch)
                        val busy = downloading == result.id
                        DenseListItem(
                            selected = false,
                            enabled = downloading == null || busy,
                            onClick = { choose(result) },
                            modifier = (if (i == 0) Modifier.focusRequester(first) else Modifier).testTag("subtitle-result:${result.id}"),
                            headlineContent = { Text(name.name, style = TvType.body, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                            supportingContent = name.note?.let { { Text(it, style = TvType.small, maxLines = 1, overflow = TextOverflow.Ellipsis) } },
                            trailingContent = if (busy) {
                                { Text(stringResource(R.string.subtitle_downloading), style = TvType.small) }
                            } else {
                                null
                            },
                            colors = ListItemStyle.colors(content = colors.onPlayer, disabled = colors.onPlayerMuted),
                            shape = ListItemStyle.shape(),
                            scale = ListItemStyle.scale(),
                            border = ListItemStyle.border(),
                        )
                    }
                }
            }
            LaunchedEffect(Unit) { focusWhenPlaced(first) }
        }
    }
}
