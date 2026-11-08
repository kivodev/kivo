package com.mmcdole.kivo.android.mobile.books

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Replay10
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.theme.LocalKivo
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.Artwork
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.core.display.timeLeft

/**
 * The mini player above the navigation bar while a book is loaded: its
 * cover, title and "Chapter 12 · 4 hr 21 min left", Back 10 seconds and Play
 * or Pause, with the book's progress as a line along its foot. A tap opens
 * Now Playing.
 */
@Composable
fun MiniPlayer(state: MiniPlayerUiState, onOpen: () -> Unit, onBack: () -> Unit, onToggle: () -> Unit, modifier: Modifier = Modifier) {
    val open = stringResource(R.string.mobile_open_now_playing)
    Box(
        modifier
            .fillMaxWidth()
            .padding(horizontal = space(2), vertical = space(1))
            .height(MiniHeight)
            .clip(MiniShape)
            .background(MaterialTheme.colorScheme.surfaceContainerHigh)
            .clickable(onClickLabel = open, onClick = onOpen)
            .testTag("miniPlayer"),
    ) {
        Row(
            Modifier.fillMaxHeight().padding(start = space(2), end = space(1)),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(space(3)),
        ) {
            Artwork(state.cover, CoverSize, Modifier.size(CoverSize).clip(RoundedCornerShape(8.dp)))
            Column(Modifier.weight(1f)) {
                Text(state.title, style = MaterialTheme.typography.titleSmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                state.chapterIndex?.let { chapter ->
                    Text(
                        stringResource(R.string.mobile_mini_line, chapter + 1, display.timeLeft(state.remainingMs)),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
            IconButton(onClick = onBack, modifier = Modifier.testTag("miniBack")) {
                Icon(Icons.Default.Replay10, stringResource(R.string.mobile_back_10))
            }
            IconButton(onClick = onToggle, modifier = Modifier.testTag("miniPlay")) {
                if (state.loading) {
                    CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                } else {
                    Icon(
                        if (state.playing) Icons.Default.Pause else Icons.Default.PlayArrow,
                        stringResource(if (state.playing) R.string.mobile_pause else R.string.mobile_play),
                    )
                }
            }
        }
        // The book's progress along the foot.
        Box(Modifier.align(Alignment.BottomStart).fillMaxWidth().height(ProgressHeight).background(LocalKivo.current.progressTrack))
        Box(
            Modifier
                .align(Alignment.BottomStart)
                .fillMaxWidth(state.fraction.coerceIn(0f, 1f))
                .height(ProgressHeight)
                .background(MaterialTheme.colorScheme.primary)
                .testTag("miniProgress"),
        )
    }
}

private val MiniHeight = 64.dp
private val MiniShape = RoundedCornerShape(16.dp)
private val CoverSize = 48.dp
private val ProgressHeight = 2.dp
