package com.mmcdole.kivo.android.detail

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.Artwork
import com.mmcdole.kivo.android.ui.CardRow
import com.mmcdole.kivo.android.ui.CardShape
import com.mmcdole.kivo.android.ui.RowCardsTop
import com.mmcdole.kivo.android.ui.RowsHoldPlace
import com.mmcdole.kivo.android.ui.EmptyState
import com.mmcdole.kivo.android.ui.Failed
import com.mmcdole.kivo.android.ui.InlineFailed
import com.mmcdole.kivo.android.ui.LocalSession
import com.mmcdole.kivo.android.ui.FocusRail
import com.mmcdole.kivo.android.ui.Loader
import com.mmcdole.kivo.android.ui.SkeletonBlock
import com.mmcdole.kivo.android.ui.SkeletonGate
import com.mmcdole.kivo.android.ui.SkeletonLine
import com.mmcdole.kivo.android.ui.SkeletonRow
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.PosterWidth
import com.mmcdole.kivo.android.ui.RowTitle
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.SafeTop
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.WorkCard
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import com.mmcdole.kivo.api.models.Person
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.data.Heading

/** A person's page's read, refetched when live events say their root works changed. */
class PersonViewModel(session: ServerSession, id: String) : ViewModel() {
    /** Their name and photo as the lists read lately named them, which show at once; null when none did. */
    val seed: Heading? = session.session.library.knownPerson(id)

    val person = Loader(viewModelScope) { session.session.library.person(id) }
}

/**
 * A person's page: their photo and name, shown at once when a list named
 * them, then their movies, shows and other videos, newest first; with none,
 * focus goes to the rail.
 */
@Composable
fun PersonScreen(session: ServerSession, id: String, navigator: Navigator) {
    val model = viewModel(key = id) { PersonViewModel(session, id) }
    val state by model.person.state.collectAsStateWithLifecycle()
    val person = state.getOrNull()
    val heading = person?.let { Heading(it.id, it.name, it.photo) } ?: model.seed
    val problem = state.problemOrNull()
    if (heading == null && problem != null) {
        Failed(problem, session.session.baseUrl, model.person::retry, session.graph::changeServer)
        return
    }
    SkeletonGate(heading, { PersonSkeleton() }) { Person(it, person, problem, model.person::retry, navigator) }
}

/** A person's page while it loads: their photo and name, then a row of posters. */
@Composable
private fun PersonSkeleton() {
    Column(Modifier.fillMaxSize().padding(PersonPadding), verticalArrangement = Arrangement.spacedBy(Kivo.space(8))) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(Kivo.space(6))) {
            SkeletonBlock(Modifier.size(160.dp), radius = KivoTokens.Radius.full)
            SkeletonLine(280.dp, TvType.title)
        }
        SkeletonRow(CardShape.Poster)
    }
}

private val PersonPadding = PaddingValues(start = 32.dp, top = SafeTop, end = SafeEdge, bottom = SafeEdge)

/** The page from [heading]; its rows once [person] is read, a row's placeholder until then, or why it couldn't be read. */
@Composable
private fun Person(heading: Heading, person: Person?, problem: RequestProblem?, onRetry: () -> Unit, navigator: Navigator) {
    val memory = rememberFocusMemory()
    val rows = listOf(
        R.string.results_movies to "movie",
        R.string.results_shows to "series",
        R.string.results_videos to "video",
    ).map { (title, kind) -> title to person?.works.orEmpty().filter { it.kind == kind } }.filter { it.second.isNotEmpty() }
    // A focused row sits where the first one starts, under the photo.
    RowsHoldPlace(SafeTop + 160.dp + Kivo.space(8) + RowCardsTop) {
        LazyColumn(
            Modifier.fillMaxSize(),
            contentPadding = PersonPadding,
            verticalArrangement = Arrangement.spacedBy(Kivo.space(8)),
        ) {
            item {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(Kivo.space(6))) {
                    Artwork(heading.image, heading.name.take(1), 1f, 160.dp, Modifier.size(160.dp), corners = KivoTokens.Radius.full)
                    Text(heading.name, style = TvType.title, color = Kivo.colors.text)
                }
            }
            when {
                person != null && rows.isEmpty() -> item { EmptyState(stringResource(R.string.person_nothing_to_watch), null) }
                person == null && problem != null -> item { InlineFailed(problem, LocalSession.current.session.baseUrl, onRetry) }
                person == null -> item { SkeletonRow(CardShape.Poster) }
            }
            for ((title, works) in rows) {
                item(key = title) {
                    Column {
                        RowTitle(stringResource(title), Modifier.padding(bottom = Kivo.space(3)))
                        CardRow {
                            items(works, key = { it.id }) { w ->
                                WorkCard(w, CardShape.Poster, PosterWidth, { navigator.push(Screen.Work(w.id)) }, Modifier.focusKey(memory, w.id))
                            }
                        }
                    }
                }
            }
        }
    }
    if (person != null && rows.isEmpty()) FocusRail()
    rows.firstOrNull()?.let { (_, works) -> LaunchedEffect(Unit) { memory.restore(works.first().id) } }
}
