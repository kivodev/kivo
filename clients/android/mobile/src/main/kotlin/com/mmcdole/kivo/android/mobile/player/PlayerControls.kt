package com.mmcdole.kivo.android.mobile.player

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.requiredSize
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.wrapContentSize
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Forward30
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PictureInPictureAlt
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Replay10
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Subtitles
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.isSpecified
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.layout.boundsInRoot
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.ProgressBarRangeInfo
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.progressBarRangeInfo
import androidx.compose.ui.semantics.setProgress
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import coil3.compose.rememberAsyncImagePainter
import coil3.request.ImageRequest
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.cast.CastButton
import com.mmcdole.kivo.android.mobile.theme.KivoTokens
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.Artwork
import com.mmcdole.kivo.android.mobile.ui.LocalSession
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.android.mobile.ui.endsAt
import com.mmcdole.kivo.api.models.Chapter
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.core.ProblemCode
import com.mmcdole.kivo.core.display.clock
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.play.PlayerFailure
import com.mmcdole.kivo.core.play.PlayerState
import com.mmcdole.kivo.core.play.failure
import com.mmcdole.kivo.core.player.MarkerKind
import com.mmcdole.kivo.core.player.SKIP_BACK_MS
import com.mmcdole.kivo.core.player.SKIP_FORWARD_MS
import com.mmcdole.kivo.core.player.chapterAt
import com.mmcdole.kivo.core.player.SkipMarker
import com.mmcdole.kivo.core.player.SpriteFrame
import com.mmcdole.kivo.core.player.UpNext
import com.mmcdole.kivo.core.player.UpNextPhase

private val colors = KivoTokens.dark

/** Where the popups rise from, in the player's coordinates: the Audio & Subtitles and Playback Settings buttons. */
class MenuAnchors {
    var origin: Rect = Rect.Zero
    var tracks: Rect = Rect.Zero
    var settings: Rect = Rect.Zero

    fun of(menu: PlayerMenu): Rect = (if (menu == PlayerMenu.Tracks) tracks else settings).translate(-origin.left, -origin.top)
}

/**
 * The top row over the video: Close, the title with an episode's series
 * and code under it, then Cast and Picture in picture.
 */
@Composable
fun PlayerTopRow(work: WorkDetail?, onClose: () -> Unit, onPictureInPicture: (() -> Unit)?, modifier: Modifier = Modifier) {
    Row(
        modifier
            .fillMaxWidth()
            .background(Brush.verticalGradient(listOf(colors.playerScrim, Color.Transparent)))
            .padding(horizontal = space(2), vertical = space(2)),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        PlayerIcon(Icons.Default.KeyboardArrowDown, stringResource(R.string.mobile_close), onClose)
        Column(Modifier.weight(1f).padding(horizontal = space(2))) {
            val episode = work?.episode
            Text(work?.title.orEmpty(), style = MaterialTheme.typography.titleMedium, color = colors.onPlayer, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (episode != null) {
                Text(
                    "${episode.seriesTitle} · ${display.episodeCode(episode)}",
                    style = MaterialTheme.typography.bodySmall,
                    color = colors.onPlayerMuted,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
        CastButton()
        if (onPictureInPicture != null) PlayerIcon(Icons.Default.PictureInPictureAlt, stringResource(R.string.mobile_picture_in_picture), onPictureInPicture)
    }
}

/**
 * The bottom of the controls: the scrub bar between the time played and
 * the time left, with when the video ends; under it back 10, play or pause
 * and forward 30 in the middle, and Audio & Subtitles and Playback Settings
 * at the right, whose places [anchors] keeps for their popups.
 */
@Composable
fun PlayerBottomBar(
    state: PlayerUiState,
    hasTracks: Boolean,
    anchors: MenuAnchors,
    onToggle: () -> Unit,
    onSkip: (Long) -> Unit,
    onScrubbing: (Boolean) -> Unit,
    onSeek: (Long) -> Unit,
    onMenu: (PlayerMenu) -> Unit,
    modifier: Modifier = Modifier,
) {
    val playback = state.playback
    val play = playback.play
    val live = playback.live
    val duration = play?.durationMs?.toLong() ?: 0
    Column(
        modifier
            .fillMaxWidth()
            .background(Brush.verticalGradient(listOf(Color.Transparent, colors.playerScrim)))
            .padding(start = space(4), end = space(4), top = space(6), bottom = space(2)),
    ) {
        ScrubBar(
            positionMs = playback.positionMs,
            durationMs = duration,
            chapters = play?.chapters.orEmpty(),
            enabled = live,
            preview = { at -> state.previews?.let { SpriteFrame.atOrNull(it.manifest, it.url, at) } },
            onScrubbing = onScrubbing,
            onSeek = onSeek,
        )
        Box(Modifier.fillMaxWidth()) {
            if (duration > 0) {
                Text(
                    endsAt(((duration - playback.positionMs) / state.speed).toLong()),
                    style = MaterialTheme.typography.bodySmall,
                    color = colors.onPlayerMuted,
                    modifier = Modifier.align(Alignment.CenterStart),
                )
            }
            Row(Modifier.align(Alignment.Center), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(space(6))) {
                PlayerIcon(Icons.Default.Replay10, stringResource(R.string.mobile_back_10), { onSkip(-SKIP_BACK_MS) }, enabled = live, size = 48.dp)
                PlayerIcon(
                    if (playback.paused) Icons.Default.PlayArrow else Icons.Default.Pause,
                    stringResource(if (playback.paused) R.string.mobile_play else R.string.mobile_pause),
                    onToggle,
                    enabled = live,
                    size = 56.dp,
                    tag = "playPause",
                )
                PlayerIcon(Icons.Default.Forward30, stringResource(R.string.mobile_forward_30), { onSkip(SKIP_FORWARD_MS) }, enabled = live, size = 48.dp)
            }
            if (play != null) {
                Row(Modifier.align(Alignment.CenterEnd)) {
                    if (hasTracks) {
                        PlayerIcon(
                            Icons.Default.Subtitles,
                            stringResource(R.string.mobile_audio_and_subtitles),
                            { onMenu(PlayerMenu.Tracks) },
                            Modifier.onGloballyPositioned { anchors.tracks = it.boundsInRoot() },
                            tag = "tracks",
                        )
                    }
                    PlayerIcon(
                        Icons.Default.Settings,
                        stringResource(R.string.mobile_playback_settings),
                        { onMenu(PlayerMenu.Settings) },
                        Modifier.onGloballyPositioned { anchors.settings = it.boundsInRoot() },
                        tag = "settings",
                    )
                }
            }
        }
    }
}

/**
 * The scrub bar: the time played and left either side, chapter marks as
 * ticks; a finger on it holds the controls up and moves a target, shown
 * above the bar with its preview frame (once the play's previews are
 * ready), time and chapter, and seeks there as it lifts.
 */
@Composable
private fun ScrubBar(
    positionMs: Long,
    durationMs: Long,
    chapters: List<Chapter>,
    enabled: Boolean,
    preview: (Long) -> SpriteFrame?,
    onScrubbing: (Boolean) -> Unit,
    onSeek: (Long) -> Unit,
) {
    var target by remember { mutableStateOf<Long?>(null) }
    val shown = target ?: positionMs
    val fraction = if (durationMs > 0) (shown.toFloat() / durationMs).coerceIn(0f, 1f) else 0f
    val scrubbing by rememberUpdatedState(onScrubbing)
    val seek by rememberUpdatedState(onSeek)
    val label = stringResource(R.string.mobile_playback_position)
    val value = stringResource(R.string.mobile_time_of, display.clock(shown), display.clock(durationMs))
    Row(verticalAlignment = Alignment.CenterVertically) {
        Text(display.clock(shown), style = MaterialTheme.typography.labelMedium, color = colors.onPlayer, modifier = Modifier.widthIn(min = 44.dp).clearAndSetSemantics {})
        BoxWithConstraints(
            Modifier
                .weight(1f)
                .height(48.dp)
                .testTag("scrub")
                // TalkBack's one control for the bar: swiping up or down moves it ten seconds.
                .clearAndSetSemantics {
                    contentDescription = label
                    stateDescription = value
                    if (enabled && durationMs > 0) {
                        progressBarRangeInfo = ProgressBarRangeInfo(shown.toFloat(), 0f..durationMs.toFloat(), steps = (durationMs / SKIP_BACK_MS).toInt().coerceAtLeast(0))
                        setProgress { ms ->
                            seek(ms.toLong().coerceIn(0, durationMs))
                            true
                        }
                    }
                }
                .pointerInput(durationMs, enabled) {
                    if (!enabled || durationMs <= 0) return@pointerInput
                    fun at(x: Float) = (x / size.width * durationMs).toLong().coerceIn(0, durationMs)
                    awaitEachGesture {
                        val down = awaitFirstDown()
                        down.consume()
                        scrubbing(true)
                        target = at(down.position.x)
                        while (true) {
                            val change = awaitPointerEvent().changes.firstOrNull { it.id == down.id } ?: break
                            if (!change.pressed) break
                            change.consume()
                            target = at(change.position.x)
                        }
                        target?.let(seek)
                        target = null
                        scrubbing(false)
                    }
                },
            contentAlignment = Alignment.CenterStart,
        ) {
            val held = target != null
            val height = if (held) 6.dp else 4.dp
            Box(Modifier.fillMaxWidth().height(height).clip(CircleShape).background(colors.playerTrack))
            Box(Modifier.fillMaxWidth(fraction).height(height).clip(CircleShape).background(colors.progress))
            if (durationMs > 0) {
                for (chapter in chapters.drop(1)) {
                    Box(Modifier.offset(x = maxWidth * (chapter.startMs.toFloat() / durationMs)).width(2.dp).height(height).background(colors.onPlayerMuted))
                }
                val thumb = if (held) 18.dp else 12.dp
                Box(Modifier.offset(x = maxWidth * fraction - thumb / 2).size(thumb).clip(CircleShape).background(colors.onPlayer))
            }
            val at = target
            if (at != null) {
                ScrubTarget(at, chapters, preview(at), maxWidth * fraction)
            }
        }
        Text(
            "−" + display.clock(durationMs - shown),
            style = MaterialTheme.typography.labelMedium,
            color = colors.onPlayer,
            textAlign = TextAlign.End,
            modifier = Modifier.widthIn(min = 52.dp).clearAndSetSemantics {},
        )
    }
}

/** Above the bar at [x]: the target's preview frame, its time and its chapter, floating so the controls don't move. */
@Composable
private fun ScrubTarget(at: Long, chapters: List<Chapter>, frame: SpriteFrame?, x: Dp) {
    val density = LocalDensity.current
    val frameWidth = with(density) { (frame?.width ?: 0).toDp() }.coerceAtLeast(64.dp)
    Column(
        Modifier
            .wrapContentSize(Alignment.BottomStart, unbounded = true)
            .offset(x = x - frameWidth / 2, y = (-24).dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(space(1)),
    ) {
        if (frame != null) PreviewFrame(frame)
        Text(
            listOfNotNull(display.clock(at), chapterAt(chapters, at)?.title).joinToString(" · "),
            style = MaterialTheme.typography.labelLarge,
            color = colors.onPlayer,
        )
    }
}

/** One preview frame, cut from its sprite sheet at the size it was made. */
@Composable
private fun PreviewFrame(frame: SpriteFrame) {
    val session = LocalSession.current
    val painter = rememberAsyncImagePainter(
        ImageRequest.Builder(LocalContext.current).data(frame.sheetUrl).size(coil3.size.Size.ORIGINAL).build(),
        imageLoader = session.images,
    )
    val sheet = painter.intrinsicSize
    with(LocalDensity.current) {
        Box(
            Modifier
                .size(frame.width.toDp(), frame.height.toDp())
                .clip(RoundedCornerShape(KivoTokens.Radius.sm))
                .background(colors.player)
                .testTag("previewFrame"),
        ) {
            if (sheet.isSpecified) {
                Image(
                    painter = painter,
                    contentDescription = null,
                    contentScale = ContentScale.None,
                    alignment = Alignment.TopStart,
                    modifier = Modifier
                        .wrapContentSize(Alignment.TopStart, unbounded = true)
                        .requiredSize(sheet.width.toDp(), sheet.height.toDp())
                        .offset { IntOffset(-frame.x, -frame.y) },
                )
            }
        }
    }
}

/** A control's glyph in a round target. */
@Composable
fun PlayerIcon(
    icon: ImageVector,
    label: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    size: Dp = 48.dp,
    tag: String? = null,
) {
    IconButton(
        onClick = onClick,
        enabled = enabled,
        modifier = modifier.size(size).then(if (tag != null) Modifier.testTag(tag) else Modifier),
        colors = IconButtonDefaults.iconButtonColors(contentColor = colors.onPlayer, disabledContentColor = colors.onPlayerMuted.copy(alpha = 0.5f)),
    ) {
        Icon(icon, label, Modifier.size(size * 0.6f))
    }
}

/** "Skip Intro", "Skip Recap" or "Skip Credits". */
@Composable
fun SkipButton(marker: SkipMarker, onSkip: () -> Unit) {
    Button(
        onClick = onSkip,
        modifier = Modifier.testTag("skip"),
        colors = ButtonDefaults.buttonColors(containerColor = colors.onPlayer, contentColor = Color.Black),
    ) {
        Text(
            stringResource(
                when (marker.kind) {
                    MarkerKind.Intro -> R.string.mobile_skip_intro
                    MarkerKind.Recap -> R.string.mobile_skip_recap
                    MarkerKind.Credits -> R.string.mobile_skip_credits
                },
            ),
        )
    }
}

/**
 * The next episode: its still, "Up next" with its countdown, or waiting
 * for Play now with the user's autoplay off, or "Still watching?"; its code
 * and title; Play now, and Hide or Watch credits.
 */
@Composable
fun UpNextCard(next: WorkDetail, upNext: UpNext, onPlayNow: () -> Unit, onHide: () -> Unit) {
    val asks = upNext.phase == UpNextPhase.StillWatching
    Row(
        Modifier
            .width(360.dp)
            .clip(RoundedCornerShape(KivoTokens.Radius.lg))
            .background(colors.playerPanel)
            .padding(space(3))
            .testTag("upNext"),
        horizontalArrangement = Arrangement.spacedBy(space(3)),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Artwork(next.images.still ?: next.images.backdrop, 120.dp, Modifier.width(120.dp).height(68.dp).clip(RoundedCornerShape(KivoTokens.Radius.md)))
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(space(1))) {
            Text(stringResource(if (asks) R.string.mobile_still_watching else R.string.mobile_up_next), style = MaterialTheme.typography.labelMedium, color = colors.onPlayerMuted)
            Text(
                listOfNotNull(next.episode?.let(display::episodeCode), next.title).joinToString(" · "),
                style = MaterialTheme.typography.titleSmall,
                color = colors.onPlayer,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            if (upNext.phase == UpNextPhase.CountingDown) {
                Text(stringResource(R.string.mobile_playing_in, upNext.secondsLeft), style = MaterialTheme.typography.bodySmall, color = colors.onPlayerMuted)
            }
            Row(horizontalArrangement = Arrangement.spacedBy(space(2))) {
                FilledTonalButton(onClick = onPlayNow, modifier = Modifier.testTag("playNow")) { Text(stringResource(R.string.mobile_play_now)) }
                OutlinedButton(onClick = onHide) { Text(stringResource(if (upNext.atCredits) R.string.mobile_watch_credits else R.string.mobile_hide)) }
            }
        }
    }
}

/** Playback stopped: what failed, the reason in words, then Try again and Back. */
@Composable
fun PlayerErrorPanel(playback: PlayerState, onRetry: () -> Unit, onBack: () -> Unit) {
    val failure = playback.failure ?: return
    val problem = failure as? PlayerFailure.Problem
    val title = when {
        failure is PlayerFailure.Ended -> R.string.mobile_playback_stopped
        problem?.knownCode == ProblemCode.StreamLimit -> R.string.mobile_stream_limit_title
        else -> R.string.mobile_couldnt_play
    }
    Box(Modifier.fillMaxSize().background(colors.playerScrim).testTag("playerError"), contentAlignment = Alignment.Center) {
        Column(Modifier.widthIn(max = 480.dp).padding(space(6)), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(space(3))) {
            Text(stringResource(title), style = MaterialTheme.typography.titleLarge, color = colors.onPlayer, textAlign = TextAlign.Center)
            Text(display.failure(failure), style = MaterialTheme.typography.bodyMedium, color = colors.onPlayerMuted, textAlign = TextAlign.Center)
            Row(horizontalArrangement = Arrangement.spacedBy(space(3))) {
                Button(onClick = onRetry) { Text(stringResource(R.string.mobile_try_again)) }
                OutlinedButton(onClick = onBack) { Text(stringResource(R.string.mobile_back)) }
            }
        }
    }
}

/** A turning ring while the video loads, buffers or seeks. */
@Composable
fun Spinner(modifier: Modifier = Modifier) {
    val turn = rememberInfiniteTransition(label = "spinner")
        .animateFloat(0f, 360f, infiniteRepeatable(tween(1_000, easing = LinearEasing)), label = "turn")
    Canvas(modifier.size(48.dp).graphicsLayer { rotationZ = turn.value }) {
        drawArc(colors.onPlayer, startAngle = 0f, sweepAngle = 270f, useCenter = false, style = Stroke(4.dp.toPx(), cap = StrokeCap.Round))
    }
}

/** A double tap's skips on one side, totalled: "−20 s" or "+30 s". */
@Composable
fun SkipBurstLabel(forward: Boolean, seconds: Int, modifier: Modifier = Modifier) {
    Box(modifier.clip(CircleShape).background(colors.playerScrim).padding(horizontal = space(4), vertical = space(2))) {
        Text(
            stringResource(if (forward) R.string.mobile_skipped_forward else R.string.mobile_skipped_back, seconds),
            style = MaterialTheme.typography.titleMedium,
            color = colors.onPlayer,
        )
    }
}
