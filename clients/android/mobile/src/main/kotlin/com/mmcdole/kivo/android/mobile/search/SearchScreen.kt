package com.mmcdole.kivo.android.mobile.search

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.CollectionRoute
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.app.PersonRoute
import com.mmcdole.kivo.android.mobile.app.handle
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.CollectionCard
import com.mmcdole.kivo.android.mobile.ui.Failed
import com.mmcdole.kivo.android.mobile.ui.Message
import com.mmcdole.kivo.android.mobile.ui.NamedCard
import com.mmcdole.kivo.android.mobile.ui.WorkCard
import com.mmcdole.kivo.android.mobile.ui.WorkMenu
import com.mmcdole.kivo.android.mobile.ui.WorkMenuHost
import com.mmcdole.kivo.android.mobile.ui.roleName
import com.mmcdole.kivo.android.mobile.ui.CardShape
import com.mmcdole.kivo.android.mobile.ui.rowCardWidth
import com.mmcdole.kivo.android.mobile.ui.wideWidth
import com.mmcdole.kivo.android.mobile.ui.shapeOf
import com.mmcdole.kivo.api.models.SearchResults
import com.mmcdole.kivo.api.models.WorkSummary

/**
 * Search: the field, then People, the works by kind and Collections, each
 * a row; the last results stay, dimmed, while the next search runs.
 */
@Composable
fun SearchScreen(session: ServerSession, nav: MobileNav) {
    val model = viewModel { SearchViewModel(session.session, session.lastSearch) }
    val state by model.state.collectAsStateWithLifecycle()
    LaunchedEffect(state.query) { session.lastSearch = state.query }
    val focus = remember { FocusRequester() }
    val focusManager = LocalFocusManager.current
    Column(Modifier.fillMaxSize().statusBarsPadding()) {
        OutlinedTextField(
            value = state.query,
            onValueChange = model::setQuery,
            modifier = Modifier.padding(horizontal = space(4), vertical = space(2)).wideWidth().fillMaxWidth().focusRequester(focus).testTag("searchField"),
            placeholder = { Text(stringResource(R.string.mobile_search_hint)) },
            leadingIcon = { Icon(Icons.Default.Search, null) },
            trailingIcon = {
                if (state.query.isNotEmpty()) IconButton(onClick = { model.setQuery("") }) { Icon(Icons.Default.Close, stringResource(R.string.mobile_clear_search)) }
            },
            singleLine = true,
            shape = CircleShape,
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
            keyboardActions = KeyboardActions(onSearch = { focusManager.clearFocus() }),
        )
        if (state.searching) LinearProgressIndicator(Modifier.fillMaxWidth()) else LinearProgressIndicator({ 0f }, Modifier.fillMaxWidth().alpha(0f))
        val answer = state.answer
        val results = answer?.results
        val problem = answer?.problem
        when {
            state.query.isBlank() -> Message(stringResource(R.string.mobile_search_empty), null)
            problem != null -> Failed(problem, stringResource(R.string.mobile_search_failed), model.serverName, model::retry)
            results == null -> Unit
            results.works.isEmpty() && results.people.isEmpty() && results.collections.isEmpty() -> Message(stringResource(R.string.mobile_no_results, answer.query), null) {
                TextButton(onClick = { model.setQuery("") }) { Text(stringResource(R.string.mobile_clear_search)) }
            }
            else -> WorkMenuHost({ action, work -> nav.handle(action, work, model.marks) }) { openMenu ->
                Results(results, nav, openMenu, Modifier.alpha(if (state.searching) 0.5f else 1f))
            }
        }
    }
    LaunchedEffect(Unit) { if (state.query.isEmpty()) focus.requestFocus() }
}

@Composable
private fun Results(results: SearchResults, nav: MobileNav, openMenu: (WorkMenu) -> Unit, modifier: Modifier) {
    val works = results.works
    val rows = listOf(
        R.string.mobile_movies to works.filter { it.kind == "movie" },
        R.string.mobile_shows to works.filter { it.kind == "series" },
        R.string.mobile_episodes to works.filter { it.kind == "episode" },
        R.string.mobile_other_videos to works.filter { it.kind == "video" || it.kind == "extra" },
        R.string.mobile_books to works.filter { it.kind == "book" },
    ).filter { it.second.isNotEmpty() }
    LazyColumn(modifier.fillMaxSize().testTag("results"), contentPadding = PaddingValues(bottom = space(8))) {
        if (results.people.isNotEmpty()) {
            item(key = "people") {
                Section(stringResource(R.string.mobile_people)) {
                    items(results.people, key = { it.id }) { person ->
                        NamedCard(
                            person.name,
                            "${roleName(person.role)} · ${pluralStringResource(R.plurals.mobile_count_titles, person.workCount, person.workCount)}",
                            person.photo,
                            88.dp,
                            round = true,
                            { nav.open(PersonRoute(person.id)) },
                        )
                    }
                }
            }
        }
        items(rows, key = { it.first }) { (title, list) -> WorksSection(stringResource(title), list, nav, openMenu) }
        if (results.collections.isNotEmpty()) {
            item(key = "collections") {
                Section(stringResource(R.string.mobile_collections)) {
                    items(results.collections, key = { it.id }) { c -> CollectionCard(c, rowCardWidth(CardShape.Poster), { nav.open(CollectionRoute(c.id)) }) }
                }
            }
        }
    }
}

@Composable
private fun WorksSection(title: String, works: List<WorkSummary>, nav: MobileNav, openMenu: (WorkMenu) -> Unit) {
    Section(title) {
        items(works, key = { it.id }) { work ->
            val shape = shapeOf(work)
            WorkCard(work, shape, rowCardWidth(shape), { nav.openWork(work) }, { openMenu(WorkMenu(work)) })
        }
    }
}

@Composable
private fun Section(title: String, content: LazyListScope.() -> Unit) {
    Column(Modifier.padding(top = space(4))) {
        Text(title, Modifier.padding(horizontal = space(4), vertical = space(2)), style = MaterialTheme.typography.titleMedium)
        LazyRow(contentPadding = PaddingValues(horizontal = space(4)), horizontalArrangement = Arrangement.spacedBy(12.dp), content = content)
    }
}
