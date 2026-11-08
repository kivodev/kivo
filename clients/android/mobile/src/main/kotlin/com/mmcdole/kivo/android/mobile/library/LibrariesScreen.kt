package com.mmcdole.kivo.android.mobile.library

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Headphones
import androidx.compose.material.icons.filled.Movie
import androidx.compose.material.icons.filled.PlayCircle
import androidx.compose.material.icons.filled.Tv
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.LibraryRoute
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.Failed
import com.mmcdole.kivo.android.mobile.ui.Loading
import com.mmcdole.kivo.android.mobile.ui.Message
import com.mmcdole.kivo.android.mobile.ui.TabTopBar
import com.mmcdole.kivo.api.models.Library

/** A library's icon by its kind. */
fun libraryIcon(kind: String): ImageVector = when (kind) {
    "shows" -> Icons.Default.Tv
    "audiobooks" -> Icons.Default.Headphones
    "videos" -> Icons.Default.PlayCircle
    else -> Icons.Default.Movie
}

/** The user's libraries: the pinned ones in their order, then More libraries, each opening its grid. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LibrariesScreen(session: ServerSession, nav: MobileNav) {
    val model = viewModel { LibrariesViewModel(session.session) }
    val state by model.state.collectAsStateWithLifecycle()
    val scroll = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    Scaffold(
        modifier = Modifier.nestedScroll(scroll.nestedScrollConnection),
        topBar = { TabTopBar(stringResource(R.string.mobile_tab_libraries), scroll) },
    ) { padding ->
        val pinned = state.pinned
        val problem = state.problem
        when {
            pinned == null && problem != null ->
                Failed(problem, stringResource(R.string.mobile_couldnt_load_libraries), state.serverName, model::retry, Modifier.padding(padding))
            pinned == null -> Loading(Modifier.padding(padding))
            pinned.isEmpty() && state.more.isEmpty() -> Message(stringResource(R.string.mobile_no_libraries), stringResource(R.string.mobile_no_libraries_sentence), Modifier.padding(padding))
            else -> LazyColumn(Modifier.fillMaxSize().testTag("libraries"), contentPadding = padding) {
                items(pinned, key = { it.id }) { LibraryRow(it, nav) }
                if (state.more.isNotEmpty()) {
                    item(key = "more") {
                        Text(
                            stringResource(R.string.mobile_more_libraries),
                            Modifier.padding(start = space(4), top = space(6), bottom = space(2)),
                            style = MaterialTheme.typography.labelLarge,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    items(state.more, key = { it.id }) { LibraryRow(it, nav) }
                }
            }
        }
    }
}

@Composable
private fun LibraryRow(library: Library, nav: MobileNav) {
    ListItem(
        headlineContent = { Text(library.name) },
        leadingContent = { Icon(libraryIcon(library.kind), null) },
        trailingContent = { Text(library.workCount.toString(), color = MaterialTheme.colorScheme.onSurfaceVariant) },
        colors = ListItemDefaults.colors(containerColor = Color.Transparent),
        modifier = Modifier.testTag("library:${library.name}").clickable { nav.open(LibraryRoute(library.id)) },
    )
}
