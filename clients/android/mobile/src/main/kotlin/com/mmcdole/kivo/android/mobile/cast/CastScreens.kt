package com.mmcdole.kivo.android.mobile.cast

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.VolumeUp
import androidx.compose.material.icons.filled.CastConnected
import androidx.compose.material.icons.filled.Forward30
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PhoneAndroid
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Replay10
import androidx.compose.material.icons.filled.Tv
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Slider
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import android.content.Context
import android.os.Bundle
import android.view.ContextThemeWrapper
import androidx.mediarouter.app.MediaRouteButton
import androidx.mediarouter.app.MediaRouteChooserDialog
import androidx.mediarouter.app.MediaRouteChooserDialogFragment
import androidx.mediarouter.app.MediaRouteControllerDialog
import androidx.mediarouter.app.MediaRouteControllerDialogFragment
import androidx.mediarouter.app.MediaRouteDialogFactory
import com.google.android.gms.cast.framework.CastButtonFactory
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.PlayerRoute
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.Artwork
import com.mmcdole.kivo.android.mobile.ui.CardCorners
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.core.display.clock
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.player.Markers

/** Casting, for the screens that show it: null where Google Play services isn't, and nothing casts. */
val LocalCast = staticCompositionLocalOf<CastViewModel?> { null }

/**
 * The Cast button: Google's MediaRouteButton, in the top bar's colour,
 * which shows only while Cast devices are found, opens Google's list of them
 * and, connected, its controller for the TV. Without Google Play services
 * there's none.
 */
@Composable
fun CastButton(modifier: Modifier = Modifier) {
    if (LocalCast.current?.offered != true) return
    AndroidView(
        factory = { context ->
            MediaRouteButton(ContextThemeWrapper(context, CAST_THEME)).also {
                it.dialogFactory = CastDialogs
                CastButtonFactory.setUpMediaRouteButton(context.applicationContext, it)
            }
        },
        modifier = modifier.size(48.dp).testTag("cast"),
    )
}

/**
 * The theme Google's Cast button and its device list and controller draw
 * in: AppCompat's dark one, which they need, over the app's own window theme.
 */
private val CAST_THEME = androidx.appcompat.R.style.Theme_AppCompat_NoActionBar

/** Google's device list and controller, in [CAST_THEME]. */
private object CastDialogs : MediaRouteDialogFactory() {
    override fun onCreateChooserDialogFragment() = CastChooserFragment()

    override fun onCreateControllerDialogFragment() = CastControllerFragment()
}

/** Google's list of Cast devices, in [CAST_THEME]; public, as the system recreates it. */
class CastChooserFragment : MediaRouteChooserDialogFragment() {
    override fun onCreateChooserDialog(context: Context, savedInstanceState: Bundle?) = MediaRouteChooserDialog(ContextThemeWrapper(context, CAST_THEME))
}

/** Google's controller for the TV connected, in [CAST_THEME]; public, as the system recreates it. */
class CastControllerFragment : MediaRouteControllerDialogFragment() {
    override fun onCreateControllerDialog(context: Context, savedInstanceState: Bundle?) = MediaRouteControllerDialog(ContextThemeWrapper(context, CAST_THEME))
}

/** "Can't reach the server.", or why the cast stopped, in words. */
@Composable
private fun CastProblem.words(): String = words(
    unreachable = stringResource(R.string.mobile_cast_unreachable),
    signedOut = stringResource(R.string.mobile_cast_signed_out),
    stopped = stringResource(R.string.mobile_playback_stopped),
    outdated = stringResource(R.string.mobile_cast_server_outdated),
    receiverOutdated = stringResource(R.string.mobile_cast_receiver_outdated),
    notPlayable = stringResource(R.string.mobile_cast_not_playable),
)

/** The movie, or the episode's series. */
private fun title(work: WorkDetail?): String = work?.episode?.seriesTitle ?: work?.title.orEmpty()

/** An episode's "S3 · E4 · Reload", or a movie's "1990 · PG-13". */
private fun line(work: WorkDetail?): String? {
    work ?: return null
    work.episode?.let { return "${display.episodeCode(it)} · ${work.title}" }
    return listOfNotNull(work.year?.toString(), work.contentRating).joinToString(" · ").ifEmpty { null }
}

/**
 * The mini controller while casting, in the book mini player's place: the
 * work's still, its title over "Casting to Living Room TV", and Play or
 * Pause. When the cast can't go on, the reason takes the device's line,
 * with Retry. A tap opens the remote.
 */
@Composable
fun CastMiniController(cast: CastViewModel, onOpen: () -> Unit) {
    val state by cast.state.collectAsStateWithLifecycle()
    Surface(color = MaterialTheme.colorScheme.surfaceContainerHigh, modifier = Modifier.fillMaxWidth().testTag("castMini")) {
        Row(
            Modifier.clickable(onClick = onOpen).padding(start = space(3), end = space(1), top = space(2), bottom = space(2)),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(space(3)),
        ) {
            Artwork(state.work?.images?.let { it.still ?: it.backdrop }, 64.dp, Modifier.size(56.dp, 32.dp).clip(CardCorners))
            Column(Modifier.weight(1f)) {
                Text(title(state.work), style = MaterialTheme.typography.titleSmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                val problem = state.problem
                if (problem != null) {
                    Text(problem.words(), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error, maxLines = 1, overflow = TextOverflow.Ellipsis)
                } else {
                    Text(stringResource(R.string.mobile_casting_to, state.device.orEmpty()), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
                }
            }
            Box(Modifier.size(48.dp), contentAlignment = Alignment.Center) {
                when {
                    state.problem != null -> IconButton(onClick = cast::retry) { Icon(Icons.Default.Refresh, stringResource(R.string.mobile_retry)) }
                    state.waiting -> CircularProgressIndicator(Modifier.size(24.dp), strokeWidth = 2.dp)
                    else -> IconButton(onClick = cast::togglePlay, Modifier.testTag("castMiniPlay")) {
                        Icon(if (state.playing) Icons.Default.Pause else Icons.Default.PlayArrow, stringResource(if (state.playing) R.string.mobile_pause else R.string.mobile_play))
                    }
                }
            }
        }
    }
}

/**
 * The remote while casting: the work's still with "Casting to Living Room
 * TV", the title and its line, the scrub bar, Back 10, Play or Pause and
 * Forward 30, the TV's volume, then Stop casting, which carries on here from
 * the TV's position, and Disconnect, which leaves the TV playing. When the
 * cast can't go on, the reason and Try again take the scrub bar's and the
 * transport's place. The chevron closes it to the mini controller, and the
 * cast plays on; once nothing plays on the TV it closes.
 */
@Composable
fun CastRemoteScreen(cast: CastViewModel, onClose: () -> Unit, onContinueHere: (PlayerRoute) -> Unit) {
    val state by cast.state.collectAsStateWithLifecycle()
    var scrubbing by remember { mutableStateOf<Float?>(null) }
    Column(
        Modifier
            .fillMaxSize()
            .background(MaterialTheme.colorScheme.background)
            .statusBarsPadding()
            .navigationBarsPadding()
            .verticalScroll(rememberScrollState())
            .testTag("castRemote"),
    ) {
        Row(Modifier.fillMaxWidth().padding(horizontal = space(1)), verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = onClose, Modifier.testTag("castRemoteClose")) { Icon(Icons.Default.KeyboardArrowDown, stringResource(R.string.mobile_close)) }
            Spacer(Modifier.weight(1f))
            CastButton()
        }
        Column(Modifier.padding(horizontal = space(6)).widthIn(max = 520.dp).align(Alignment.CenterHorizontally), horizontalAlignment = Alignment.CenterHorizontally) {
            Artwork(state.work?.images?.let { it.still ?: it.backdrop }, 520.dp, Modifier.fillMaxWidth().aspectRatio(16f / 9f).clip(MaterialTheme.shapes.large))
            Row(Modifier.padding(top = space(5)), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(space(2))) {
                Icon(Icons.Default.CastConnected, null, Modifier.size(16.dp), tint = MaterialTheme.colorScheme.primary)
                Text(stringResource(R.string.mobile_casting_to, state.device.orEmpty()), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary)
            }
            Text(title(state.work), Modifier.padding(top = space(5)), style = MaterialTheme.typography.headlineSmall, textAlign = TextAlign.Center)
            line(state.work)?.let { Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center) }
            Spacer(Modifier.height(space(6)))
            val problem = state.problem
            if (problem != null) {
                Text(problem.words(), color = MaterialTheme.colorScheme.error, textAlign = TextAlign.Center)
                Button(onClick = cast::retry, Modifier.padding(top = space(3))) { Text(stringResource(R.string.mobile_try_again)) }
            } else {
                val duration = state.durationMs.coerceAtLeast(1)
                Slider(
                    value = scrubbing ?: (state.positionMs.toFloat() / duration).coerceIn(0f, 1f),
                    onValueChange = { scrubbing = it },
                    onValueChangeFinished = {
                        scrubbing?.let { cast.seek((it * duration).toLong()) }
                        scrubbing = null
                    },
                    enabled = state.media != null,
                    modifier = Modifier.testTag("castScrub"),
                )
                Row(Modifier.fillMaxWidth()) {
                    val shown = scrubbing?.let { (it * duration).toLong() } ?: state.positionMs
                    Text(display.clock(shown), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Spacer(Modifier.weight(1f))
                    Text(display.clock(state.durationMs), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Row(Modifier.padding(top = space(4)), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(space(6))) {
                    IconButton(onClick = { cast.skip(-Markers.BACK_MS) }) { Icon(Icons.Default.Replay10, stringResource(R.string.mobile_back_10), Modifier.size(32.dp)) }
                    Box(Modifier.size(72.dp), contentAlignment = Alignment.Center) {
                        if (state.waiting) {
                            CircularProgressIndicator()
                        } else {
                            FilledIconButton(onClick = cast::togglePlay, Modifier.size(72.dp).testTag("castPlay")) {
                                Icon(if (state.playing) Icons.Default.Pause else Icons.Default.PlayArrow, stringResource(if (state.playing) R.string.mobile_pause else R.string.mobile_play), Modifier.size(40.dp))
                            }
                        }
                    }
                    IconButton(onClick = { cast.skip(Markers.FORWARD_MS) }) { Icon(Icons.Default.Forward30, stringResource(R.string.mobile_forward_30), Modifier.size(32.dp)) }
                }
            }
            Row(Modifier.fillMaxWidth().padding(top = space(6)), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(space(3))) {
                Icon(Icons.AutoMirrored.Filled.VolumeUp, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
                Slider(value = state.volume, onValueChange = cast::setVolume, modifier = Modifier.weight(1f).testTag("castVolume"))
            }
        }
        Spacer(Modifier.height(space(4)))
        RemoteRow(Icons.Default.PhoneAndroid, stringResource(R.string.mobile_stop_casting), stringResource(R.string.mobile_continue_on_phone), "stopCasting") {
            cast.stopCasting()?.let(onContinueHere) ?: onClose()
        }
        RemoteRow(Icons.Default.Tv, stringResource(R.string.mobile_disconnect), stringResource(R.string.mobile_keep_playing_on_tv), "disconnect") {
            cast.disconnect()
            onClose()
        }
    }
}

@Composable
private fun RemoteRow(icon: ImageVector, title: String, detail: String, tag: String, onClick: () -> Unit) {
    ListItem(
        headlineContent = { Text(title) },
        supportingContent = { Text(detail) },
        leadingContent = { Icon(icon, null) },
        colors = ListItemDefaults.colors(containerColor = Color.Transparent),
        modifier = Modifier.testTag(tag).clickable(onClick = onClick),
    )
}
