package com.mmcdole.kivo.android.mobile.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Cast
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LargeTopAppBar
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.TopAppBarScrollBehavior
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.mmcdole.kivo.android.mobile.R

/** Cast, which comes later: its place in the top bar. */
@Composable
fun CastButton() {
    IconButton(onClick = {}, enabled = false) { Icon(Icons.Default.Cast, stringResource(R.string.mobile_cast)) }
}

/** The signed-in user's initial on an accent circle, opening Settings. */
@Composable
fun AvatarButton(initial: String, onClick: () -> Unit) {
    IconButton(onClick = onClick, modifier = Modifier.testTag("avatar")) {
        Box(Modifier.size(32.dp).background(MaterialTheme.colorScheme.primary, CircleShape), contentAlignment = Alignment.Center) {
            Text(initial, color = MaterialTheme.colorScheme.onPrimary, style = MaterialTheme.typography.labelLarge)
        }
    }
}

/** A tab's top bar: its large title, collapsing as the page scrolls, with Cast and, on Home, the avatar. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TabTopBar(title: String, scroll: TopAppBarScrollBehavior, initial: String? = null, onAvatar: () -> Unit = {}, actions: @Composable () -> Unit = {}) {
    LargeTopAppBar(
        title = { Text(title, maxLines = 1, overflow = TextOverflow.Ellipsis) },
        actions = {
            actions()
            CastButton()
            if (initial != null) AvatarButton(initial, onAvatar)
        },
        scrollBehavior = scroll,
        colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background, scrolledContainerColor = MaterialTheme.colorScheme.background),
    )
}

/** A pushed page's top bar: back, its title, and its actions. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PageTopBar(title: String, onBack: () -> Unit, scroll: TopAppBarScrollBehavior? = null, large: Boolean = false, actions: @Composable () -> Unit = {}) {
    val colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.background, scrolledContainerColor = MaterialTheme.colorScheme.background)
    val navigation = @Composable {
        IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, stringResource(R.string.mobile_back)) }
    }
    if (large) {
        LargeTopAppBar(
            title = { Text(title, maxLines = 1, overflow = TextOverflow.Ellipsis) },
            navigationIcon = navigation,
            actions = { actions() },
            scrollBehavior = scroll,
            colors = colors,
        )
    } else {
        TopAppBar(
            title = { Text(title, maxLines = 1, overflow = TextOverflow.Ellipsis) },
            navigationIcon = navigation,
            actions = { actions() },
            scrollBehavior = scroll,
            colors = colors,
        )
    }
}
