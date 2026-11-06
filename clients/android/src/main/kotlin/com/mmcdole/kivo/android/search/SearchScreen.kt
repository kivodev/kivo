package com.mmcdole.kivo.android.search

import android.content.Intent
import android.speech.RecognizerIntent
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.tv.material3.ClickableSurfaceDefaults
import androidx.tv.material3.Surface
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.main.RailWidth
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.home.RowCardsBelow
import com.mmcdole.kivo.android.home.RowTitlePadding
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.CardRow
import com.mmcdole.kivo.android.ui.CardShape
import com.mmcdole.kivo.android.ui.CollectionCard
import com.mmcdole.kivo.android.ui.RowsHoldPlace
import com.mmcdole.kivo.android.ui.EmptyState
import com.mmcdole.kivo.android.ui.FocusMemory
import com.mmcdole.kivo.android.ui.KivoButton
import com.mmcdole.kivo.android.ui.KivoIcon
import com.mmcdole.kivo.android.ui.KivoIconView
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.PersonCard
import com.mmcdole.kivo.android.ui.RowTitle
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.SafeTop
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.TextInput
import com.mmcdole.kivo.android.ui.WorkCard
import com.mmcdole.kivo.android.ui.display
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.Lit
import com.mmcdole.kivo.android.ui.raisedOnFocus
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import com.mmcdole.kivo.android.ui.roleLabel
import com.mmcdole.kivo.api.models.SearchResults
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.display.problem
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** The keyboard column's width: six keys across. */
private val KeyboardWidth = 275.dp

/** The gap between the keyboard and the results. */
private val ColumnGap = 28.dp

/** How far in the results start in their column, which leaves a focused card's glow clear of the keyboard. */
private val ResultsStart = 14.dp

/** Where the results' cards start on the screen. */
internal val ResultsLeft = RailWidth + 24.dp + KeyboardWidth + ColumnGap + ResultsStart

/** What the results say when there are none, centred inside the screen's margin. */
private val InsideMargin = Modifier.padding(end = SafeEdge)

/** The gap between the results' rows. */
private val ResultRowGap = 12.dp

internal val ResultPosterWidth = 101.dp

internal val ResultLandscapeWidth = 231.dp

/** A key's size, and the gap between keys. */
private val KeyWidth = 40.dp
private val KeyHeight = 38.5.dp
private val KeyGap = 7.dp

/** The keys in the last line, which share the keyboard's width. */
private fun lastLineKeyWidth(keys: Int) = (KeyboardWidth - KeyGap * (keys - 1)) / keys

/** Search's rows of works, always in this order under People, by the kinds they hold; the TV shows no books. */
private enum class ResultRow(val title: Int, val kinds: Set<String>, val shape: CardShape) {
    Movies(R.string.results_movies, setOf("movie"), CardShape.Poster),
    Shows(R.string.results_shows, setOf("series"), CardShape.Poster),
    Episodes(R.string.results_episodes, setOf("episode"), CardShape.Landscape),
    Videos(R.string.results_videos, setOf("video", "extra"), CardShape.Landscape),
    ;

    /** Four posters or two landscape cards across the results. */
    val cardWidth get() = if (shape == CardShape.Landscape) ResultLandscapeWidth else ResultPosterWidth
}

/** Search, opening on the last search typed while this server is open, searched again at once. */
class SearchViewModel(private val session: ServerSession) : ViewModel() {
    var query by mutableStateOf(session.lastSearch)
        private set
    var results by mutableStateOf<SearchResults?>(null)
        private set
    var searching by mutableStateOf(false)
        private set
    var failed by mutableStateOf<RequestProblem?>(null)
        private set
    private var job: Job? = null

    init {
        search(query, pauseMs = 0)
    }

    /** Searches again after a failure. */
    fun retry() = search(query, pauseMs = 0)

    /** Searches once typing pauses for about 350 ms, keeping the last results meanwhile. */
    fun type(text: String) {
        query = text
        session.lastSearch = text
        search(text, pauseMs = 350)
    }

    private fun search(text: String, pauseMs: Long) {
        job?.cancel()
        if (text.isBlank()) {
            results = null
            searching = false
            return
        }
        job = viewModelScope.launch {
            delay(pauseMs)
            searching = true
            try {
                results = session.session.library.search(text.trim(), 50)
                failed = null
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                failed = RequestProblem.of(e)
            }
            searching = false
        }
    }
}

/**
 * Search: the platform keyboard in the field, or the
 * on-screen grid keyboard beside the results, with voice input; the people who match in a
 * row, then a row of results per kind, then the collections that match.
 */
@Composable
fun SearchScreen(session: ServerSession, navigator: Navigator) {
    val model = viewModel { SearchViewModel(session) }
    val memory = rememberFocusMemory()
    Row(Modifier.fillMaxSize().padding(start = 24.dp, top = SafeTop, bottom = SafeTop)) {
        Column(Modifier.width(KeyboardWidth), verticalArrangement = Arrangement.spacedBy(21.dp)) {
            TextInput(
                label = stringResource(R.string.search),
                value = model.query,
                onValueChange = model::type,
                labelShown = false,
                icon = KivoIcon.Search,
                minHeight = 43.dp,
            )
            GridKeyboard(model.query, model::type, memory)
        }
        Spacer(Modifier.width(ColumnGap))
        // Its rows run on to the screen's edge, scrolling inside themselves.
        Box(Modifier.weight(1f).fillMaxHeight()) {
            val results = model.results
            val failed = model.failed
            when {
                failed != null -> EmptyState(stringResource(R.string.couldnt_load), display.problem(failed), InsideMargin) {
                    KivoButton(stringResource(R.string.try_again), model::retry, Modifier.testTag("tryAgain"), primary = true)
                }
                results == null -> EmptyState(stringResource(R.string.search_prompt), null, InsideMargin)
                results.works.isEmpty() && results.people.isEmpty() && results.collections.isEmpty() && !model.searching -> EmptyState(stringResource(R.string.no_results_for, model.query), null, InsideMargin) {
                    KivoButton(stringResource(R.string.clear_search), { model.type("") })
                }
                else -> RowsHoldPlace(RowCardsBelow) {
                    LazyColumn(
                        Modifier.fillMaxSize().alpha(if (model.searching) 0.6f else 1f),
                        contentPadding = PaddingValues(start = ResultsStart),
                        verticalArrangement = Arrangement.spacedBy(ResultRowGap),
                    ) {
                        if (results.people.isNotEmpty()) {
                            item(key = "people") {
                                Column(Modifier.raisedOnFocus()) {
                                    RowTitle(stringResource(R.string.results_people), Modifier.padding(RowTitlePadding))
                                    CardRow {
                                        items(results.people, key = { it.id }) { person ->
                                            val titles = pluralStringResource(R.plurals.titles, person.workCount, person.workCount)
                                            PersonCard(
                                                person.name,
                                                person.photo,
                                                stringResource(R.string.person_titles, roleLabel(person.role), titles),
                                                { navigator.push(Screen.Person(person.id)) },
                                                Modifier.focusKey(memory, "person/${person.id}"),
                                            )
                                        }
                                    }
                                }
                            }
                        }
                        for (row in ResultRow.entries) {
                            val works = results.works.filter { it.kind in row.kinds }
                            if (works.isEmpty()) continue
                            item(key = row.name) {
                                Column(Modifier.raisedOnFocus()) {
                                    RowTitle(stringResource(row.title), Modifier.padding(RowTitlePadding))
                                    CardRow {
                                        items(works, key = { it.id }) { work ->
                                            WorkCard(work, row.shape, row.cardWidth, { navigator.push(Screen.Work(work.id)) }, Modifier.focusKey(memory, work.id))
                                        }
                                    }
                                }
                            }
                        }
                        if (results.collections.isNotEmpty()) {
                            item(key = "collections") {
                                Column(Modifier.raisedOnFocus()) {
                                    RowTitle(stringResource(R.string.collections), Modifier.padding(RowTitlePadding))
                                    CardRow {
                                        items(results.collections, key = { it.id }) { c ->
                                            CollectionCard(c, ResultPosterWidth, { navigator.push(Screen.Collection(c.id)) }, Modifier.focusKey(memory, "collection/${c.id}"))
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    // The grid keyboard first, so the platform keyboard opens only when the field is chosen.
    LaunchedEffect(Unit) { memory.restore("key/a") }
}

private val keys = (('a'..'z') + ('0'..'9')).map { it.toString() }

/**
 * The on-screen grid keyboard: letters, digits, space, delete and clear, and
 * the microphone where the TV has voice input, which searches for what's said.
 */
@Composable
private fun GridKeyboard(query: String, onType: (String) -> Unit, memory: FocusMemory) {
    val context = LocalContext.current
    val voice = remember { voiceIntent().takeIf { it.resolveActivity(context.packageManager) != null } }
    val listen = rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
        result.data?.getStringArrayListExtra(RecognizerIntent.EXTRA_RESULTS)?.firstOrNull()?.let(onType)
    }
    Column(verticalArrangement = Arrangement.spacedBy(KeyGap)) {
        for (row in keys.chunked(6)) {
            Row(horizontalArrangement = Arrangement.spacedBy(KeyGap)) {
                for (key in row) Key(Modifier.width(KeyWidth).focusKey(memory, "key/$key"), key) { onType(query + key) }
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(KeyGap)) {
            val width = lastLineKeyWidth(if (voice != null) 4 else 3)
            Key(Modifier.width(width).focusKey(memory, "key/space"), stringResource(R.string.key_space)) { onType("$query ") }
            Key(Modifier.width(width).focusKey(memory, "key/delete"), null, KivoIcon.ChevronLeft, tag = "delete") { onType(query.dropLast(1)) }
            Key(Modifier.width(width).focusKey(memory, "key/clear"), stringResource(R.string.key_clear)) { onType("") }
            if (voice != null) {
                Key(Modifier.width(width).focusKey(memory, "key/voice"), null, KivoIcon.Mic, tag = "voice") { listen.launch(voice) }
            }
        }
    }
}

/** Speech input for a search, as the TV's own voice search takes it. */
private fun voiceIntent() = Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH)
    .putExtra(RecognizerIntent.EXTRA_LANGUAGE_MODEL, RecognizerIntent.LANGUAGE_MODEL_WEB_SEARCH)

@Composable
private fun Key(modifier: Modifier, label: String?, icon: KivoIcon? = null, tag: String? = null, onClick: () -> Unit) {
    val colors = Kivo.colors
    Surface(
        onClick = onClick,
        modifier = modifier.height(KeyHeight).testTag("key:${tag ?: label}"),
        shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(KivoTokens.Radius.sm)),
        colors = ClickableSurfaceDefaults.colors(
            containerColor = colors.surface,
            contentColor = colors.text,
            focusedContainerColor = Lit.container,
            focusedContentColor = Lit.content,
        ),
        scale = ClickableSurfaceDefaults.scale(focusedScale = KivoTokens.Tv.focusScale),
    ) {
        Box(Modifier.fillMaxWidth().height(KeyHeight), contentAlignment = Alignment.Center) {
            if (icon != null) KivoIconView(icon, size = 18.dp) else Text(label.orEmpty(), style = TvType.bodyStrong, maxLines = 1)
        }
    }
}
