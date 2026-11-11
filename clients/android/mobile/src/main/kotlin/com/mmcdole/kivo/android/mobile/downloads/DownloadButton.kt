package com.mmcdole.kivo.android.mobile.downloads

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.DownloadForOffline
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Stop
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.ActionLabel
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.api.models.DownloadOption
import com.mmcdole.kivo.core.display.size
import com.mmcdole.kivo.core.downloads.DownloadRefusedException
import com.mmcdole.kivo.core.downloads.DownloadStatus
import com.mmcdole.kivo.core.downloads.downloadProblem
import com.mmcdole.kivo.core.downloads.downloadQuality
import com.mmcdole.kivo.core.settings.DownloadQuality

/** What a detail page's Download button does, by its state's own words. */
class DownloadActions(
    val download: (DownloadQuality?) -> Unit,
    val pause: () -> Unit,
    val resume: () -> Unit,
    val retry: () -> Unit,
    val remove: () -> Unit,
    val problemSeen: () -> Unit,
)

/**
 * A detail page's Download button, round beside My List and Watched. Before
 * a download, a video's opens the qualities with their sizes, the setting's
 * checked, and a book downloads its original at once; a work that can't be
 * downloaded here says why. Once asked for, it shows where the download is:
 * a dashed ring while the server is asked, then a ring while it's prepared
 * and comes, a check when it's ready and a warning when it failed, and opens
 * what can be done with it. The first download asks to show its notification.
 */
@Composable
fun DownloadButton(state: DownloadUiState, book: Boolean, actions: DownloadActions, modifier: Modifier = Modifier) {
    var sheet by remember { mutableStateOf(false) }
    var refused by remember { mutableStateOf<String?>(null) }
    val ask = rememberNotificationAsk()
    val item = state.item
    val estimate = state.estimate
    val label = stringResource(R.string.mobile_download)
    Column(
        modifier
            .widthIn(min = 72.dp)
            .clip(MaterialTheme.shapes.medium)
            .clickable(enabled = !state.asking) {
                when {
                    item != null -> sheet = true
                    estimate != null && !estimate.playable -> refused = estimate.reason?.let { display.reason(it) } ?: display.downloadProblem(DownloadRefusedException(DownloadStatus.NOT_PLAYABLE))
                    book -> {
                        ask()
                        actions.download(null)
                    }
                    else -> sheet = true
                }
            }
            .padding(vertical = space(2))
            .testTag("download"),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Box(Modifier.size(24.dp), contentAlignment = Alignment.Center) { Glyph(state) }
        ActionLabel(label, MaterialTheme.colorScheme.onSurface, Modifier.padding(top = space(1)))
    }
    if (sheet) {
        if (item == null) {
            QualitySheet(state, onDismiss = { sheet = false }) { quality ->
                ask()
                actions.download(quality)
            }
        } else {
            ManageSheet(state, actions, onDismiss = { sheet = false })
        }
    }
    (refused ?: state.problem)?.let { problem ->
        val seen = {
            refused = null
            actions.problemSeen()
        }
        AlertDialog(
            onDismissRequest = seen,
            text = { Text(problem) },
            confirmButton = { TextButton(onClick = seen) { Text(stringResource(R.string.mobile_ok)) } },
        )
    }
}

/** The button's glyph: an arrow to download, a dashed ring while it's asked for, a ring filling as it's prepared and comes, a check, or a warning. */
@Composable
private fun Glyph(state: DownloadUiState) {
    val item = state.item
    val progress = item?.let { DownloadWords.progress(it) }
    when {
        item == null && state.asking -> DownloadRing(0f, dashed = true, size = 22.dp, modifier = Modifier.testTag("asking"))
        item == null -> Icon(Icons.Default.Download, null)
        item.status == DownloadStatus.Ready -> Icon(Icons.Default.DownloadForOffline, stringResource(R.string.mobile_downloaded), tint = MaterialTheme.colorScheme.primary)
        item.status is DownloadStatus.Failed -> Icon(Icons.Default.ErrorOutline, stringResource(R.string.mobile_download_failed), tint = MaterialTheme.colorScheme.error)
        progress != null -> ProgressRing(
            progress,
            center = if (item.status is DownloadStatus.Paused) Icons.Default.Download else Icons.Default.Stop.takeIf { progress.stage != Stage.Queued },
            size = 22.dp,
            modifier = Modifier.testTag("progress"),
        )
    }
}

/** The qualities to download at, each with its size, the setting's checked: Original, High and Standard. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun QualitySheet(state: DownloadUiState, onDismiss: () -> Unit, onPick: (DownloadQuality?) -> Unit) {
    ModalBottomSheet(onDismissRequest = onDismiss, modifier = Modifier.testTag("qualitySheet")) {
        SheetTitle(stringResource(R.string.mobile_download))
        val options = state.estimate?.options
        if (options == null) {
            Box(Modifier.fillMaxWidth().padding(space(6)), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        } else {
            for (option in options) {
                OptionRow(option, checked = option.quality == state.setting?.value) {
                    onDismiss()
                    onPick(DownloadQuality.parseOrNull(option.quality))
                }
            }
        }
        Box(Modifier.padding(bottom = space(6)))
    }
}

/** A quality: its name, and its size, "Up to 4.1 GB" where the server converts it. */
@Composable
private fun OptionRow(option: DownloadOption, checked: Boolean, note: String? = null, onClick: () -> Unit) {
    val size = display.size(option.sizeBytes)
    ListItem(
        headlineContent = { Text(DownloadQuality.parseOrNull(option.quality)?.let(display::downloadQuality) ?: option.quality) },
        supportingContent = { Text(note ?: if (option.asIs) size else stringResource(R.string.mobile_up_to_size, size)) },
        trailingContent = { if (checked) Icon(Icons.Default.Check, null, tint = MaterialTheme.colorScheme.primary) },
        colors = ListItemDefaults.colors(containerColor = Color.Transparent),
        modifier = Modifier.testTag("quality:${option.quality}").clickable(onClick = onClick),
    )
}

/** A download asked for: where it is, then Pause or Resume while it comes, Retry when it failed, and Remove download. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ManageSheet(state: DownloadUiState, actions: DownloadActions, onDismiss: () -> Unit) {
    val item = state.item ?: return
    ModalBottomSheet(onDismissRequest = onDismiss, modifier = Modifier.testTag("downloadSheet")) {
        SheetTitle(item.title)
        Text(
            listOfNotNull(downloadLine(item), item.quality?.let(display::downloadQuality)).joinToString(" · "),
            Modifier.padding(horizontal = space(6)).padding(bottom = space(2)),
            style = MaterialTheme.typography.bodyMedium,
            color = if (item.status is DownloadStatus.Failed) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
        )
        fun run(action: () -> Unit) {
            onDismiss()
            action()
        }
        when (item.status) {
            is DownloadStatus.Failed -> SheetRow(Icons.Default.Refresh, stringResource(R.string.mobile_retry), "retry") { run(actions.retry) }
            is DownloadStatus.Paused -> SheetRow(Icons.Default.Download, stringResource(R.string.mobile_resume_download), "resume") { run(actions.resume) }
            is DownloadStatus.Downloading, is DownloadStatus.Waiting -> SheetRow(Icons.Default.Pause, stringResource(R.string.mobile_pause_download), "pause") { run(actions.pause) }
            else -> Unit
        }
        SheetRow(Icons.Default.Delete, stringResource(R.string.mobile_remove_download), "remove", danger = true) { run(actions.remove) }
        Box(Modifier.padding(bottom = space(6)))
    }
}

/**
 * Downloads a season's or the series' episodes: each quality with the
 * episodes' total size, those already here or on their way left as they are.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun EpisodesSheet(state: DownloadUiState, onPick: (String) -> Unit, onDismiss: () -> Unit) {
    val sheet = state.episodes ?: return
    val ask = rememberNotificationAsk()
    ModalBottomSheet(onDismissRequest = onDismiss, modifier = Modifier.testTag("episodesSheet")) {
        SheetTitle(sheet.season?.let { stringResource(R.string.mobile_download_named, it.title) } ?: stringResource(R.string.mobile_download_series))
        val estimates = sheet.estimates
        if (estimates == null) {
            Box(Modifier.fillMaxWidth().padding(space(6)), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        } else {
            val wanted = estimates.filter { it.playable && it.workId !in state.downloaded }
            for (option in sheet.qualities(wanted)) {
                val total = wanted.sumOf { e -> e.options.firstOrNull { it.quality == option.quality }?.sizeBytes ?: 0 }
                val asIs = wanted.all { e -> e.options.firstOrNull { it.quality == option.quality }?.asIs == true }
                val size = display.size(total)
                OptionRow(option, checked = option.quality == state.setting?.value, note = if (asIs) size else stringResource(R.string.mobile_up_to_size, size)) {
                    ask()
                    onPick(option.quality)
                }
            }
            Text(
                pluralStringResource(R.plurals.mobile_episodes_to_download, wanted.size, wanted.size),
                Modifier.padding(horizontal = space(6), vertical = space(2)),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Box(Modifier.padding(bottom = space(6)))
    }
}

/** Under a detail page's actions, once a download is asked for: "Downloaded" when it's ready, else where it is. */
@Composable
fun DownloadBadge(state: DownloadUiState) {
    val item = state.item ?: return
    val failed = item.status is DownloadStatus.Failed
    Row(
        Modifier
            .clip(CircleShape)
            .background(MaterialTheme.colorScheme.surfaceContainerHigh)
            .padding(horizontal = space(3), vertical = space(1))
            .testTag("downloadBadge"),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(space(1)),
    ) {
        val tint = if (failed) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant
        Icon(if (item.status == DownloadStatus.Ready) Icons.Default.DownloadForOffline else Icons.Default.Download, null, Modifier.size(16.dp), tint = tint)
        Text(downloadBadge(item), style = MaterialTheme.typography.labelMedium, color = tint)
    }
}

@Composable
private fun SheetTitle(text: String) {
    Text(text, Modifier.padding(horizontal = space(6), vertical = space(2)), style = MaterialTheme.typography.titleMedium)
}

@Composable
private fun SheetRow(icon: ImageVector, label: String, tag: String, danger: Boolean = false, onClick: () -> Unit) {
    val color = if (danger) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface
    ListItem(
        headlineContent = { Text(label, color = color) },
        leadingContent = { Icon(icon, null, tint = color) },
        colors = ListItemDefaults.colors(containerColor = Color.Transparent),
        modifier = Modifier.testTag(tag).clickable(onClick = onClick),
    )
}

/**
 * Asks to show notifications, as a download starts, so its progress can be
 * seen from outside the app; asked only while it isn't allowed, on Android 13
 * and later, where the system stops asking once it's been refused twice.
 */
@Composable
fun rememberNotificationAsk(): () -> Unit {
    val context = LocalContext.current
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {}
    return remember(context, launcher) {
        {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
                ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
            ) {
                launcher.launch(Manifest.permission.POST_NOTIFICATIONS)
            }
        }
    }
}
