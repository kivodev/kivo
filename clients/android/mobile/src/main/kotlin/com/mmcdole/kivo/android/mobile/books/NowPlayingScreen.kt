package com.mmcdole.kivo.android.mobile.books

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.detectVerticalDragGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.MenuBook
import androidx.compose.material.icons.automirrored.filled.QueueMusic
import androidx.compose.material.icons.filled.Bedtime
import androidx.compose.material.icons.filled.Forward30
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Replay10
import androidx.compose.material.icons.filled.SkipNext
import androidx.compose.material.icons.filled.SkipPrevious
import androidx.compose.material.icons.filled.StopCircle
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.app.WorkRoute
import com.mmcdole.kivo.android.mobile.theme.LocalKivo
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.Artwork
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.api.models.Chapter
import com.mmcdole.kivo.core.books.BookPlayerModel
import com.mmcdole.kivo.core.books.BookProgress
import com.mmcdole.kivo.core.books.SleepChoice
import com.mmcdole.kivo.core.books.SleepState
import com.mmcdole.kivo.core.display.clock
import com.mmcdole.kivo.core.display.runtime
import com.mmcdole.kivo.core.play.failure

/**
 * Now Playing, over the app: the large cover, the title and "Author · read
 * by Narrator"; the chapter playing over a seek bar that spans it, with the
 * time played and left in it, and under it where the chapter is in the book
 * with the time left; Previous chapter, Back 10 seconds, Play or Pause,
 * Forward 30 seconds and Next chapter; then Speed, the sleep timer and the
 * chapters. More has Go to book and Stop listening. A swipe down, Close or
 * Back closes it; it closes by itself once the book stops.
 */
@Composable
fun NowPlayingScreen(session: ServerSession, nav: MobileNav) {
    val model = viewModel { NowPlayingViewModel(session.books.model) }
    val state by model.state.collectAsStateWithLifecycle()
    LaunchedEffect(state.shown) { if (!state.shown) nav.back() }
    var chaptersShown by rememberSaveable { mutableStateOf(false) }
    val close by rememberUpdatedState(nav::back)
    Column(
        Modifier
            .fillMaxSize()
            .background(MaterialTheme.colorScheme.background)
            .windowInsetsPadding(WindowInsets.safeDrawing)
            .pointerInput(Unit) {
                val far = 96.dp.toPx()
                var pulled = 0f
                detectVerticalDragGestures(onDragStart = { pulled = 0f }, onDragEnd = { if (pulled > far) close() }) { _, dy -> pulled += dy }
            }
            .padding(horizontal = space(6))
            .testTag("nowPlaying"),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Header(onClose = nav::back, onGoToBook = state.work?.let { { nav.replaceNowPlaying(WorkRoute(it.id)) } }, onStop = model::stop)
        BoxWithConstraints(Modifier.weight(1f).fillMaxWidth(), contentAlignment = Alignment.Center) {
            val side = minOf(maxWidth, maxHeight).coerceAtMost(360.dp)
            Artwork(state.work?.images?.poster, side, Modifier.size(side).clip(RoundedCornerShape(16.dp)))
        }
        Spacer(Modifier.height(space(5)))
        Text(
            state.work?.title.orEmpty(),
            style = MaterialTheme.typography.titleLarge,
            textAlign = TextAlign.Center,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        credit(state)?.let {
            Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center, maxLines = 2)
        }
        Spacer(Modifier.height(space(5)))
        ChapterSeek(state.progress, state.chapters, model::seekInChapter)
        state.failure?.let { Text(display.failure(it), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error) }
        Spacer(Modifier.height(space(4)))
        Transport(state, model)
        Spacer(Modifier.height(space(4)))
        Row(Modifier.fillMaxWidth().padding(bottom = space(4)), horizontalArrangement = Arrangement.SpaceEvenly, verticalAlignment = Alignment.CenterVertically) {
            SpeedButton(state.speed, model::setSpeed)
            SleepButton(state, model)
            IconButton(onClick = { chaptersShown = true }, modifier = Modifier.testTag("chapters")) {
                Icon(Icons.AutoMirrored.Filled.QueueMusic, stringResource(R.string.mobile_chapters))
            }
        }
    }
    if (chaptersShown) ChapterSheet(state, onPlay = model::playChapter, onDismiss = { chaptersShown = false })
}

/** Close at the left, and More with Go to book and Stop listening at the right. */
@Composable
private fun Header(onClose: () -> Unit, onGoToBook: (() -> Unit)?, onStop: () -> Unit) {
    var more by remember { mutableStateOf(false) }
    Row(Modifier.fillMaxWidth().padding(vertical = space(2)), verticalAlignment = Alignment.CenterVertically) {
        IconButton(onClick = onClose, modifier = Modifier.testTag("closeNowPlaying")) {
            Icon(Icons.Default.KeyboardArrowDown, stringResource(R.string.mobile_close))
        }
        Text(stringResource(R.string.mobile_now_playing), Modifier.weight(1f), style = MaterialTheme.typography.titleSmall, textAlign = TextAlign.Center)
        Box {
            IconButton(onClick = { more = true }, modifier = Modifier.testTag("nowPlayingMore")) {
                Icon(Icons.Default.MoreVert, stringResource(R.string.mobile_more))
            }
            DropdownMenu(expanded = more, onDismissRequest = { more = false }) {
                if (onGoToBook != null) {
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.mobile_go_to_book)) },
                        leadingIcon = { Icon(Icons.AutoMirrored.Filled.MenuBook, null) },
                        onClick = {
                            more = false
                            onGoToBook()
                        },
                    )
                }
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.mobile_stop_listening)) },
                    leadingIcon = { Icon(Icons.Default.StopCircle, null) },
                    onClick = {
                        more = false
                        onStop()
                    },
                )
            }
        }
    }
}

/** "Andy Weir · read by Ray Porter". */
@Composable
private fun credit(state: NowPlayingUiState): String? {
    val book = state.work?.book ?: return null
    val authors = book.authors.joinToString(", ") { it.name }.ifEmpty { null }
    val narrators = book.narrators.joinToString(", ") { it.name }.ifEmpty { null }?.let { stringResource(R.string.mobile_read_by, it) }
    return listOfNotNull(authors, narrators).joinToString(" · ").ifEmpty { null }
}

/**
 * The chapter's title over its seek bar, which spans only the chapter
 * playing, with the time played and left in it either side; a finger on
 * it moves a target within the chapter, and it seeks there as it lifts.
 * Under it where the chapter is in the book, and the time left in the book.
 */
@Composable
private fun ChapterSeek(progress: BookProgress?, chapters: List<Chapter>, onSeek: (index: Int, offsetMs: Long) -> Unit) {
    var target by remember { mutableStateOf<Pair<Int, Long>?>(null) }
    val seek by rememberUpdatedState(onSeek)
    val index = target?.first ?: progress?.chapterIndex ?: 0
    val duration = progress?.chapterDurationMs ?: 0
    val elapsed = target?.second ?: progress?.chapterElapsedMs ?: 0
    val fraction = if (duration > 0) (elapsed.toFloat() / duration).coerceIn(0f, 1f) else 0f
    Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally) {
        Text(
            progress?.let { chapterName(it.chapterTitle, it.chapterIndex) } ?: " ",
            style = MaterialTheme.typography.titleSmall,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        BoxWithConstraints(
            Modifier
                .fillMaxWidth()
                .height(32.dp)
                .testTag("chapterSeek")
                .pointerInput(progress?.chapterIndex, duration) {
                    if (progress == null || duration <= 0) return@pointerInput
                    val chapter = progress.chapterIndex
                    fun at(x: Float) = (x / size.width * duration).toLong().coerceIn(0, duration)
                    awaitEachGesture {
                        val down = awaitFirstDown()
                        down.consume()
                        target = chapter to at(down.position.x)
                        while (true) {
                            val change = awaitPointerEvent().changes.firstOrNull { it.id == down.id } ?: break
                            if (!change.pressed) break
                            change.consume()
                            target = chapter to at(change.position.x)
                        }
                        target?.let { (i, offset) -> seek(i, offset) }
                        target = null
                    }
                },
            contentAlignment = Alignment.CenterStart,
        ) {
            val track = LocalKivo.current.progressTrack
            Box(Modifier.fillMaxWidth().height(4.dp).clip(CircleShape).background(track))
            Box(Modifier.fillMaxWidth(fraction).height(4.dp).clip(CircleShape).background(MaterialTheme.colorScheme.primary))
            val thumb = if (target != null) 18.dp else 12.dp
            Box(Modifier.offset(x = maxWidth * fraction - thumb / 2).size(thumb).clip(CircleShape).background(MaterialTheme.colorScheme.onSurface))
        }
        Row(Modifier.fillMaxWidth()) {
            val muted = MaterialTheme.colorScheme.onSurfaceVariant
            Text(display.clock(elapsed), style = MaterialTheme.typography.labelMedium, color = muted)
            Spacer(Modifier.weight(1f))
            Text("−" + display.clock(duration - elapsed), style = MaterialTheme.typography.labelMedium, color = muted)
        }
        if (progress != null) {
            Spacer(Modifier.height(space(2)))
            Text(
                stringResource(R.string.mobile_book_line, index + 1, progress.chapterCount, display.runtime(progress.remainingMs)),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** A chapter's title, or "Chapter 12" for one without. */
@Composable
private fun chapterName(title: String?, index: Int): String = title?.takeIf { it.isNotBlank() } ?: stringResource(R.string.mobile_chapter_number, index + 1)

/** Previous chapter, Back 10 seconds, Play or Pause, Forward 30 seconds and Next chapter. */
@Composable
private fun Transport(state: NowPlayingUiState, model: NowPlayingViewModel) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceEvenly, verticalAlignment = Alignment.CenterVertically) {
        TransportButton(Icons.Default.SkipPrevious, stringResource(R.string.mobile_previous_chapter), "previousChapter", 32.dp, model::previousChapter)
        TransportButton(Icons.Default.Replay10, stringResource(R.string.mobile_back_10), "back10", 36.dp, model::skipBack)
        Box(
            Modifier
                .size(72.dp)
                .clip(CircleShape)
                .background(MaterialTheme.colorScheme.primary)
                .clickable(onClick = model::togglePlay)
                .testTag("nowPlayingPlay"),
            contentAlignment = Alignment.Center,
        ) {
            if (state.loading) {
                CircularProgressIndicator(Modifier.size(28.dp), color = MaterialTheme.colorScheme.onPrimary, strokeWidth = 3.dp)
            } else {
                Icon(
                    if (state.playing) Icons.Default.Pause else Icons.Default.PlayArrow,
                    stringResource(if (state.playing) R.string.mobile_pause else R.string.mobile_play),
                    Modifier.size(40.dp),
                    tint = MaterialTheme.colorScheme.onPrimary,
                )
            }
        }
        TransportButton(Icons.Default.Forward30, stringResource(R.string.mobile_forward_30), "forward30", 36.dp, model::skipForward)
        TransportButton(Icons.Default.SkipNext, stringResource(R.string.mobile_next_chapter), "nextChapter", 32.dp, model::nextChapter)
    }
}

@Composable
private fun TransportButton(icon: androidx.compose.ui.graphics.vector.ImageVector, label: String, tag: String, size: Dp, onClick: () -> Unit) {
    IconButton(onClick = onClick, modifier = Modifier.size(size + 16.dp).testTag(tag)) {
        Icon(icon, label, Modifier.size(size))
    }
}

/** The user's speed for books, 0.5× to 3×, the same in every app. */
@Composable
private fun SpeedButton(speed: Double, onSpeed: (Double) -> Unit) {
    var open by remember { mutableStateOf(false) }
    Box {
        TextButton(onClick = { open = true }, modifier = Modifier.testTag("speed")) {
            Text(speedLabel(speed), style = MaterialTheme.typography.titleSmall)
        }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            for (s in BookPlayerModel.SPEEDS) {
                DropdownMenuItem(
                    text = { Text(speedLabel(s), fontWeight = if (s == speed) FontWeight.SemiBold else null) },
                    onClick = {
                        open = false
                        onSpeed(s)
                    },
                )
            }
        }
    }
}

/** "1.25×". */
private fun speedLabel(speed: Double): String = "%s×".format(if (speed % 1.0 == 0.0) speed.toInt().toString() else speed.toString())

/** The sleep timer: the time left beside the moon while it runs; its menu offers the last choice first, then Off and the rest. */
@Composable
private fun SleepButton(state: NowPlayingUiState, model: NowPlayingViewModel) {
    var open by remember { mutableStateOf(false) }
    val running = state.sleep as? SleepState.Running
    Box {
        TextButton(onClick = { open = true }, modifier = Modifier.testTag("sleep")) {
            Icon(Icons.Default.Bedtime, stringResource(R.string.mobile_sleep_timer))
            if (running != null) {
                Spacer(Modifier.width(space(1)))
                Text(display.clock(running.remainingMs), style = MaterialTheme.typography.labelLarge)
            }
        }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            val choices = SleepChoice.MINUTES.map { SleepChoice.Minutes(it) } + SleepChoice.EndOfChapter
            val ordered = listOfNotNull(state.lastSleep) + choices.filter { it != state.lastSleep }
            if (running != null) {
                DropdownMenuItem(text = { Text(stringResource(R.string.mobile_off)) }, onClick = {
                    open = false
                    model.stopSleep()
                })
            }
            for (choice in ordered) {
                DropdownMenuItem(text = { Text(sleepLabel(choice)) }, onClick = {
                    open = false
                    model.setSleep(choice)
                })
            }
        }
    }
}

@Composable
private fun sleepLabel(choice: SleepChoice): String = when (choice) {
    is SleepChoice.Minutes -> stringResource(R.string.mobile_sleep_minutes, choice.minutes)
    SleepChoice.EndOfChapter -> stringResource(R.string.mobile_end_of_chapter)
}

/** The chapters, the one playing marked; a tap plays from its start. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ChapterSheet(state: NowPlayingUiState, onPlay: (Int) -> Unit, onDismiss: () -> Unit) {
    val current = state.progress?.chapterIndex ?: 0
    ModalBottomSheet(onDismissRequest = onDismiss, modifier = Modifier.testTag("chapterSheet")) {
        Text(stringResource(R.string.mobile_chapters), Modifier.padding(horizontal = space(6), vertical = space(2)), style = MaterialTheme.typography.titleMedium)
        val list = rememberLazyListState(initialFirstVisibleItemIndex = (current - 2).coerceAtLeast(0))
        LazyColumn(state = list, contentPadding = PaddingValues(bottom = space(6))) {
            itemsIndexed(state.chapters) { index, chapter ->
                val playing = index == current
                ListItem(
                    leadingContent = { Text("${index + 1}", Modifier.widthIn(min = 24.dp), color = MaterialTheme.colorScheme.onSurfaceVariant) },
                    headlineContent = {
                        Text(chapterName(chapter.title, index), fontWeight = if (playing) FontWeight.SemiBold else null, color = if (playing) MaterialTheme.colorScheme.primary else Color.Unspecified)
                    },
                    trailingContent = { Text(display.clock((chapter.endMs - chapter.startMs).toLong()), color = MaterialTheme.colorScheme.onSurfaceVariant) },
                    colors = ListItemDefaults.colors(containerColor = Color.Transparent),
                    modifier = Modifier.clickable {
                        onPlay(index)
                        onDismiss()
                    },
                )
            }
        }
    }
}
