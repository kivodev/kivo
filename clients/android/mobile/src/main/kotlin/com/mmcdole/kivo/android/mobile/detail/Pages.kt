package com.mmcdole.kivo.android.mobile.detail

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.app.handle
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.Artwork
import com.mmcdole.kivo.android.mobile.ui.CardCorners
import com.mmcdole.kivo.android.mobile.ui.CardShape
import com.mmcdole.kivo.android.mobile.ui.Failed
import com.mmcdole.kivo.android.mobile.ui.Loading
import com.mmcdole.kivo.android.mobile.ui.PageTopBar
import com.mmcdole.kivo.android.mobile.ui.WorkCard
import com.mmcdole.kivo.android.mobile.ui.WorkMenu
import com.mmcdole.kivo.android.mobile.ui.WorkMenuHost
import com.mmcdole.kivo.android.mobile.ui.rowCardWidth
import com.mmcdole.kivo.android.mobile.ui.shapeOf
import com.mmcdole.kivo.api.models.CollectionSource
import com.mmcdole.kivo.api.models.ImageRef
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkSummary

/** A collection: its poster, name and count, then its works. */
@Composable
fun CollectionScreen(session: ServerSession, id: String, nav: MobileNav) {
    val model = viewModel { PageViewModel.collection(session.session, id) }
    val state by model.state.collectAsStateWithLifecycle()
    val collection = state.value
    PageFrame(state, model, nav) { openMenu ->
        WorksGrid(collection?.items.orEmpty(), nav, openMenu) {
            Heading(collection?.name ?: state.name, collection?.images?.poster ?: state.image, round = false) {
                collection?.items?.size?.let { pluralStringResource(if (collection.source == CollectionSource.Provider) R.plurals.mobile_count_movies else R.plurals.mobile_count_items, it, it) }
            }
        }
    }
}

/** A book series: its cover, name and count, then its books in order. */
@Composable
fun BookSeriesScreen(session: ServerSession, id: String, nav: MobileNav) {
    val model = viewModel { PageViewModel.bookSeries(session.session, id) }
    val state by model.state.collectAsStateWithLifecycle()
    val series = state.value
    PageFrame(state, model, nav) { openMenu ->
        WorksGrid(series?.items.orEmpty(), nav, openMenu) {
            Heading(series?.name ?: state.name, series?.items?.firstOrNull()?.images?.poster ?: state.image, round = false) {
                series?.items?.size?.let { pluralStringResource(R.plurals.mobile_count_books, it, it) }
            }
        }
    }
}

/**
 * A person: their photo, name and overview, then rows of their movies,
 * shows and other videos, each book series of theirs, their other books,
 * and the books they narrated.
 */
@Composable
fun PersonScreen(session: ServerSession, id: String, nav: MobileNav) {
    val model = viewModel { PageViewModel.person(session.session, id) }
    val state by model.state.collectAsStateWithLifecycle()
    val person = state.value
    val names = PersonRowNames(
        stringResource(R.string.mobile_movies),
        stringResource(R.string.mobile_shows),
        stringResource(R.string.mobile_other_videos),
        stringResource(if (person?.bookSeries.isNullOrEmpty()) R.string.mobile_books else R.string.mobile_other_books),
        stringResource(R.string.mobile_narrated),
    )
    PageFrame(state, model, nav) { openMenu ->
        LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(bottom = space(8))) {
            item(key = "heading") {
                Heading(person?.name ?: state.name, person?.photo ?: state.image, round = true) { null }
                person?.overview?.let {
                    Text(it, Modifier.padding(horizontal = space(4), vertical = space(2)), style = MaterialTheme.typography.bodyMedium, maxLines = 6)
                }
            }
            if (person != null) {
                val rows = buildList {
                    add(names.movies to person.works.filter { it.kind == WorkKind.Movie })
                    add(names.shows to person.works.filter { it.kind == WorkKind.Series })
                    add(names.otherVideos to person.works.filter { it.kind != WorkKind.Movie && it.kind != WorkKind.Series })
                    person.bookSeries.forEach { add(it.name to it.items) }
                    add(names.books to person.standalone)
                    add(names.narrated to person.narrated)
                }.filter { it.second.isNotEmpty() }
                items(rows, key = { it.first }) { (title, works) -> WorksRow(title, works, nav, openMenu) }
            }
        }
    }
}

/** A person's rows' headings. */
private data class PersonRowNames(val movies: String, val shows: String, val otherVideos: String, val books: String, val narrated: String)

/** A page's frame: its top bar, then [content] once there's something to show, else why there isn't. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun <T : Any> PageFrame(state: PageUiState<T>, model: PageViewModel<T>, nav: MobileNav, content: @Composable (open: (WorkMenu) -> Unit) -> Unit) {
    Scaffold(topBar = { PageTopBar("", nav::back) }) { padding ->
        Box(Modifier.padding(padding).fillMaxSize()) {
            val problem = state.problem
            when {
                state.value == null && problem != null -> Failed(problem, stringResource(R.string.mobile_couldnt_load_page), state.name.orEmpty(), model::retry)
                state.value == null && state.name == null -> Loading()
                else -> WorkMenuHost({ action, menu -> nav.handle(action, menu, model.marks) }) { open -> content(open) }
            }
        }
    }
}

@Composable
private fun Heading(name: String?, image: ImageRef?, round: Boolean, detail: @Composable () -> String?) {
    Column(Modifier.padding(space(4)), horizontalAlignment = Alignment.CenterHorizontally) {
        Box(Modifier.width(120.dp).aspectRatio(if (round) 1f else CardShape.Poster.aspect).clip(if (round) CircleShape else CardCorners)) {
            Artwork(image, 120.dp, Modifier.matchParentSize())
        }
        Text(name.orEmpty(), Modifier.padding(top = space(3)), style = MaterialTheme.typography.headlineSmall, textAlign = TextAlign.Center)
        detail()?.let { Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant) }
    }
}

/** Works in a grid of 104 dp cards, under [header]. */
@Composable
private fun WorksGrid(works: List<WorkSummary>, nav: MobileNav, openMenu: (WorkMenu) -> Unit, header: @Composable () -> Unit) {
    LazyVerticalGrid(
        columns = GridCells.Adaptive(104.dp),
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(start = space(4), end = space(4), bottom = space(8)),
        horizontalArrangement = Arrangement.spacedBy(10.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        item(key = "header", span = { GridItemSpan(maxLineSpan) }) { header() }
        items(works, key = { it.id }) { work ->
            WorkCard(work, shapeOf(work), 120.dp, { nav.openWork(work) }, { openMenu(WorkMenu(work)) }, Modifier.fillMaxWidth(), captionLines = 1)
        }
    }
}

/** A titled row of works' cards. */
@Composable
private fun WorksRow(title: String, works: List<WorkSummary>, nav: MobileNav, openMenu: (WorkMenu) -> Unit) {
    Column(Modifier.padding(top = space(4))) {
        Text(title, Modifier.padding(horizontal = space(4), vertical = space(2)), style = MaterialTheme.typography.titleMedium)
        LazyRow(contentPadding = PaddingValues(horizontal = space(4)), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            items(works, key = { it.id }) { work ->
                val shape = shapeOf(work)
                WorkCard(work, shape, rowCardWidth(shape), { nav.openWork(work) }, { openMenu(WorkMenu(work)) })
            }
        }
    }
}
