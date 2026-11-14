package com.mmcdole.kivo.android.player

import android.graphics.Color as AndroidColor
import android.view.KeyEvent
import android.view.ViewGroup
import androidx.activity.compose.BackHandler
import androidx.activity.compose.LocalActivity
import androidx.annotation.OptIn
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.requiredSize
import androidx.compose.foundation.layout.wrapContentSize
import androidx.compose.ui.geometry.isSpecified
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.IntOffset
import coil3.compose.rememberAsyncImagePainter
import coil3.request.ImageRequest
import com.mmcdole.kivo.core.ProblemCode
import com.mmcdole.kivo.core.data.SubtitleSearch
import com.mmcdole.kivo.core.player.SKIP_BACK_MS
import com.mmcdole.kivo.core.player.SKIP_FORWARD_MS
import com.mmcdole.kivo.core.player.chapterAt
import com.mmcdole.kivo.core.player.SpriteFrame
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.Stable
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.platform.LocalWindowInfo
import androidx.compose.ui.layout.boundsInWindow
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.compose.ui.window.Dialog
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.media3.common.util.UnstableApi
import androidx.media3.ui.CaptionStyleCompat
import androidx.media3.ui.PlayerView
import androidx.media3.ui.SubtitleView
import androidx.tv.material3.ClickableSurfaceDefaults
import androidx.tv.material3.Surface
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.Artwork
import com.mmcdole.kivo.android.ui.labelledTitle
import com.mmcdole.kivo.android.ui.Choice
import com.mmcdole.kivo.android.ui.ChoiceMarker
import com.mmcdole.kivo.android.ui.ChoiceList
import com.mmcdole.kivo.android.ui.MenuPopup
import com.mmcdole.kivo.android.ui.menuAbove
import com.mmcdole.kivo.android.ui.menuWidth
import com.mmcdole.kivo.android.ui.SubtitleSearchPanel
import com.mmcdole.kivo.android.ui.TrackChoices
import com.mmcdole.kivo.android.ui.TrackColumns
import com.mmcdole.kivo.android.ui.columns
import com.mmcdole.kivo.android.ui.tracksMenuWidth
import com.mmcdole.kivo.android.ui.choosable
import com.mmcdole.kivo.android.ui.endsAt
import com.mmcdole.kivo.core.display.bitrate
import com.mmcdole.kivo.core.display.clock
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.display.quality
import com.mmcdole.kivo.core.display.size
import com.mmcdole.kivo.android.ui.KivoButton
import com.mmcdole.kivo.android.ui.KivoIcon
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.SafeTop
import com.mmcdole.kivo.android.ui.KeyAction
import com.mmcdole.kivo.android.ui.KivoIconView
import com.mmcdole.kivo.android.ui.LocalKeyDispatcher
import com.mmcdole.kivo.android.ui.LocalPanelSurface
import com.mmcdole.kivo.android.ui.LocalSession
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.OkKeys
import com.mmcdole.kivo.android.ui.panelProperties
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.rememberKeyDispatcher
import com.mmcdole.kivo.android.ui.rememberPanelKeys
import com.mmcdole.kivo.android.ui.focusRing
import com.mmcdole.kivo.android.ui.Lit
import com.mmcdole.kivo.android.ui.focusWhenPlaced
import com.mmcdole.kivo.android.ui.display
import com.mmcdole.kivo.api.models.Chapter
import com.mmcdole.kivo.api.models.CapabilityMismatch
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.core.player.MarkerKind
import com.mmcdole.kivo.core.player.SkipMarker
import com.mmcdole.kivo.core.player.UpNext
import com.mmcdole.kivo.core.player.UpNextPhase
import com.mmcdole.kivo.core.play.PlayerFailure
import com.mmcdole.kivo.core.play.failure
import com.mmcdole.kivo.core.play.PlayerState
import com.mmcdole.kivo.core.play.PlayerStatus
import com.mmcdole.kivo.core.session.PlayerExit
import com.mmcdole.kivo.core.settings.Quality
import com.mmcdole.kivo.android.settings.SubtitleBackground
import com.mmcdole.kivo.android.settings.SubtitleSize
import kotlinx.coroutines.delay

/** A scrub seeks to its target after this long without another step. */
private const val SCRUB_COMMIT_MS = 1_000L

/**
 * The player: the video with its subtitles, controls that show on
 * any input and hide after 5 seconds while playing, the remote map,
 * Audio & Subtitles, Playback Settings and the stream info, Skip Intro, Recap and Credits, up next
 * for episodes, and the error with its recovery actions.
 */
@Composable
fun PlayerScreen(session: ServerSession, args: Screen.Player, navigator: Navigator) {
    val model = viewModel { PlayerViewModel(session, args) }
    val state by model.controller.state.collectAsStateWithLifecycle()
    val user by session.session.user.collectAsStateWithLifecycle()
    val current = state.play
    val work = model.screen.work
    val version = work?.versions?.firstOrNull { it.id == current?.versionId }
    val episode = work?.episode
    val live = state.live

    // Leaving the app stops the play; coming back starts it again where it was.
    LifecycleEventEffect(Lifecycle.Event.ON_STOP) { model.controller.stopOnExit() }
    LifecycleEventEffect(Lifecycle.Event.ON_START) { model.controller.restore() }
    // The screen stays on while the video plays or loads; paused, the screensaver may start.
    val view = LocalView.current
    val awake = when (state.status) {
        PlayerStatus.Loading, PlayerStatus.Buffering, PlayerStatus.Seeking -> true
        PlayerStatus.Playing -> !state.paused
        else -> false
    }
    DisposableEffect(awake) {
        view.keepScreenOn = awake
        onDispose { view.keepScreenOn = false }
    }

    // Frame-rate matching, from the version's video track.
    val activity = LocalActivity.current
    val frameRate = version?.tracks?.firstOrNull { it.type == TrackType.Video }?.video?.frameRate
    val matching = model.screenSettings?.matchesFrameRate == true
    DisposableEffect(activity, frameRate, matching) {
        val matcher = activity?.let(::FrameRateMatcher)
        if (matching && frameRate != null) matcher?.match(frameRate.toFloat())
        onDispose { matcher?.release() }
    }

    // Done, the player goes where the core says: the next episode, the series or back.
    LaunchedEffect(model.screen.exit) {
        when (val exit = model.screen.exit) {
            is PlayerExit.Next -> navigator.replace(Screen.Player(exit.choice))
            is PlayerExit.Series -> navigator.returnTo(Screen.Work(exit.seriesId))
            PlayerExit.Back -> navigator.pop()
            null -> {}
        }
    }
    LaunchedEffect(Unit) {
        for (screen in model.replacements) navigator.replace(screen)
    }

    // The controls, Skip and Up next show by the core's rules, from the inputs given it.
    val video = model.video
    val ui = model.screen.ui
    val visible = ui.controlsShown
    // The menus and panels open over the video, the one showing last: Back goes to the one before it.
    var menus by remember { mutableStateOf(emptyList<PlayerMenu>()) }
    val menu = menus.lastOrNull()
    val anchors = remember { MenuAnchors() }
    val root = remember { FocusRequester() }
    val scrubBar = remember { FocusRequester() }
    val playButton = remember { FocusRequester() }
    val tracksButton = remember { FocusRequester() }
    // Where focus goes as the controls show, when the key that showed them chose; else the scrub bar.
    var revealFocus by remember { mutableStateOf<FocusRequester?>(null) }

    val showUpNext = model.screen.next != null && ui.upNext.shows
    val stillWatching = showUpNext && ui.upNext.phase == UpNextPhase.StillWatching
    val marker = ui.skipMarker
    // A menu or panel open holds the controls up and up next's countdown.
    LaunchedEffect(menu != null) { video.setMenuOpen(open = menu != null) }

    fun reveal(focus: FocusRequester? = null) {
        if (!video.screen.value.ui.controlsShown) revealFocus = focus
        video.showControls()
    }

    // Where Left and Right have moved the scrub bar to; a pause or OK seeks there, and hiding the controls drops it.
    var scrubTarget by remember { mutableStateOf<Long?>(null) }
    fun moveScrubTarget(deltaMs: Long) {
        reveal()
        val duration = current?.durationMs?.toLong() ?: return
        if (!live) return
        scrubTarget = ((scrubTarget ?: state.positionMs) + deltaMs).coerceIn(0, duration)
    }
    fun scrub(direction: Int, repeatCount: Int) = moveScrubTarget(direction * scrubStepMs(repeatCount))
    fun commitScrub(): Boolean {
        val target = scrubTarget ?: return false
        scrubTarget = null
        model.controller.seek(target)
        return true
    }
    LaunchedEffect(scrubTarget) {
        if (scrubTarget == null) return@LaunchedEffect
        delay(SCRUB_COMMIT_MS)
        commitScrub()
    }

    /** Back 10 or forward 30 seconds: from the scrub target while there is one, else from the position. */
    fun skipBy(deltaMs: Long) {
        if (scrubTarget != null) return moveScrubTarget(deltaMs)
        reveal()
        model.controller.skip(deltaMs)
    }

    fun hide() {
        video.hideControls()
        scrubTarget = null
    }
    LaunchedEffect(visible) {
        if (!visible) scrubTarget = null
    }

    fun perform(command: PlayerCommand, repeatCount: Int) {
        val c = model.controller
        when (command) {
            PlayerCommand.TogglePlay -> { reveal(); c.togglePlay() }
            PlayerCommand.Play -> { reveal(); c.resume() }
            PlayerCommand.Pause -> { reveal(); c.pause() }
            PlayerCommand.SkipBack -> skipBy(-SKIP_BACK_MS)
            PlayerCommand.SkipForward -> skipBy(SKIP_FORWARD_MS)
            PlayerCommand.ScrubBack -> scrub(-1, repeatCount)
            PlayerCommand.ScrubForward -> scrub(1, repeatCount)
            PlayerCommand.Previous -> model.playPrevious()
            PlayerCommand.Next -> video.playNext()
            PlayerCommand.ShowControls -> reveal()
            PlayerCommand.FocusPlay -> reveal(playButton)
            PlayerCommand.FocusTracks -> reveal(tracksButton)
            PlayerCommand.Skip -> video.skipMarker()
            PlayerCommand.HideControls -> hide()
            PlayerCommand.CloseMenu -> menus = menus.dropLast(1)
            PlayerCommand.Exit -> navigator.pop()
            PlayerCommand.Info -> {
                reveal()
                menus = if (menu == PlayerMenu.Info) emptyList() else listOf(PlayerMenu.Info)
            }
        }
    }

    val failed = state.status == PlayerStatus.Error && state.failure != null
    val keys = rememberKeyDispatcher { code ->
        // Any input means someone is watching: up next may play on again, and the controls wait again to hide.
        video.userActivity()
        val overlay = when {
            showUpNext -> Overlay.UpNext
            marker != null -> Overlay.Skip
            else -> Overlay.None
        }
        val command = playerCommand(code, RemoteState(visible, overlay, menuOpen = menu != null, loading = !live))
        // A key the shown controls take keeps them up.
        if (command == null && visible) reveal()
        command?.let { KeyAction { repeatCount -> perform(it, repeatCount) } }
    }

    /**
     * Opens [opened] from its button, or from the row of the menu it opens in:
     * it takes focus, so the press that opened it ends there.
     */
    fun openMenu(opened: PlayerMenu) {
        keys.focusMoved()
        menus = menus + opened
    }

    // A remote's focus: the scrub bar whenever the controls appear, or the media goes live under them, unless the
    // key that showed them chose; Up next or Skip when it appears over hidden controls or they hide under it, and
    // "Still watching?" as it appears, whatever the controls; else the player.
    val skipButton = remember { FocusRequester() }
    val playNow = remember { FocusRequester() }
    val overlayButton = when {
        showUpNext -> playNow
        marker != null -> skipButton
        else -> null
    }
    val lastShown = remember { LastShown() }
    LaunchedEffect(visible, live, overlayButton, stillWatching) {
        val was = lastShown.focus
        lastShown.focus = RemoteFocus(visible, live, overlayButton, stillWatching)
        val chosen = revealFocus
        revealFocus = null
        val target = when {
            stillWatching && !was.stillWatching -> playNow
            // Until the media is live, the scrub bar can't take focus, and the player keeps it.
            visible && (!was.visible || live && !was.live) -> chosen ?: if (live) scrubBar else root
            // The focused Skip or Up next went away under the controls.
            visible -> scrubBar.takeIf { overlayButton == null && was.overlay != null }
            overlayButton != null -> overlayButton.takeIf { was.visible || was.overlay != overlayButton }
            else -> root
        } ?: return@LaunchedEffect
        keys.focusMoved()
        focusWhenPlaced(target)
    }

    // Back reaches the back dispatcher only when nothing in the player has focus: a whole press, as it's released.
    BackHandler(enabled = !failed) {
        keys.down(KeyEvent.KEYCODE_BACK, repeatCount = 0)
        keys.up(KeyEvent.KEYCODE_BACK)
    }
    Box(
        Modifier
            .fillMaxSize()
            .background(Kivo.colors.player)
            .focusRequester(root)
            .focusable()
            .onPreviewKeyEvent { event -> !failed && keys.dispatch(event.nativeKeyEvent) },
    ) {
        Video(model.player, model.screenSettings?.subtitleSize, model.screenSettings?.subtitleBackground)

        if (state.status == PlayerStatus.Loading || state.status == PlayerStatus.Buffering || state.status == PlayerStatus.Seeking) {
            Spinner(Modifier.align(Alignment.Center))
        }

        if (visible && !failed) {
            Controls(
                title = when {
                    work == null -> ""
                    episode != null -> listOf(episode.seriesTitle, display.episodeCode(episode), work.title).joinToString(" · ")
                    else -> labelledTitle(work.title, work.movie)
                },
                state = state,
                play = current,
                speed = model.speed,
                hasEpisodes = episode != null,
                hasPrevious = episode?.previousId != null,
                hasNext = episode?.nextId != null,
                scrubTarget = scrubTarget,
                scrubBar = scrubBar,
                playButton = playButton,
                tracksButton = tracksButton,
                hasTracks = current?.let { tracksPanelShows(model, it) } == true,
                preview = { at -> model.screen.previews?.let { SpriteFrame.atOrNull(it.manifest, it.url, at) } },
                onToggle = { model.controller.togglePlay() },
                onBack = { model.controller.skip(-SKIP_BACK_MS) },
                onForward = { model.controller.skip(SKIP_FORWARD_MS) },
                onScrub = ::scrub,
                onCommitScrub = ::commitScrub,
                onPrevious = model::playPrevious,
                onNext = video::playNext,
                onMenu = ::openMenu,
                anchors = anchors,
                covered = (menu == PlayerMenu.Info || menu == PlayerMenu.SubtitleSearch) && current != null,
            )
        }

        // Above the scrub bar, in from the screen's cropped edges as the controls are.
        Column(Modifier.align(Alignment.BottomEnd).padding(end = SafeEdge, bottom = SafeTop + 107.dp), horizontalAlignment = Alignment.End) {
            if (showUpNext) {
                UpNext(model, ui.upNext, playNow)
                Spacer(Modifier.height(Kivo.space(3)))
            }
            if (marker != null) SkipButton(marker, skipButton, video::skipMarker)
        }

        if (failed) {
            val other = work?.versions?.firstOrNull { it.available && it.id != (current?.versionId ?: args.choice.versionId) }
            PlayerError(
                state = state,
                admin = user?.admin == true,
                onRetry = { model.controller.retry() },
                onPlayOther = other?.let { { model.playVersion(it.id) } },
                onBack = { navigator.pop() },
            )
        }
    }

    // While a choice starts a new play, the state keeps the play it replaces, so the open panel and its focus stay put.
    if (current != null && !failed) {
        // The menus' windows hand their keys to the player's dispatcher, and stand opaque over the video.
        CompositionLocalProvider(LocalKeyDispatcher provides keys, LocalPanelSurface provides Kivo.colors.playerPanel.copy(alpha = 1f)) {
            Menus(model, current, menus, anchors, onOpen = ::openMenu, onBack = { menus = menus.dropLast(1) }, onClose = { menus = emptyList() })
        }
    }
}

/** The video and its subtitles: Media3's view, Kivo's text style for plain subtitles, libass over it for styled ones. */
@OptIn(UnstableApi::class)
@Composable
private fun Video(player: LoadedPlayer?, size: SubtitleSize?, background: SubtitleBackground?) {
    val box = Kivo.colors.subtitleBox.toArgb()
    AndroidView(
        modifier = Modifier.fillMaxSize(),
        factory = { context ->
            PlayerView(context).apply {
                useController = false
                isFocusable = false
                descendantFocusability = ViewGroup.FOCUS_BLOCK_DESCENDANTS
                setShutterBackgroundColor(AndroidColor.BLACK)
            }
        },
        update = { view ->
            if (view.player !== player?.player) {
                view.player = player?.player
                val subtitles = view.subtitleView ?: return@AndroidView
                for (i in subtitles.childCount - 1 downTo 0) {
                    val child = subtitles.getChildAt(i)
                    if (child is io.github.peerless2012.ass.media.widget.AssSubtitleView) subtitles.removeViewAt(i)
                }
                player?.let { subtitles.addView(io.github.peerless2012.ass.media.widget.AssSubtitleView(view.context, it.ass)) }
            }
            view.subtitleView?.apply {
                // Clear of a TV's overscan at the bottom.
                setBottomPaddingFraction(0.1f)
                setFractionalTextSize(
                    when (size) {
                        SubtitleSize.Small -> 0.045f
                        SubtitleSize.Large -> 0.07f
                        else -> SubtitleView.DEFAULT_TEXT_SIZE_FRACTION
                    },
                )
                setStyle(
                    CaptionStyleCompat(
                        AndroidColor.WHITE,
                        if (background == SubtitleBackground.Box) box else AndroidColor.TRANSPARENT,
                        AndroidColor.TRANSPARENT,
                        if (background == SubtitleBackground.None || background == SubtitleBackground.Box) CaptionStyleCompat.EDGE_TYPE_NONE else CaptionStyleCompat.EDGE_TYPE_DROP_SHADOW,
                        AndroidColor.BLACK,
                        null,
                    ),
                )
            }
        },
    )
}

/**
 * The title and the transport, inside the margins of a TV that crops its
 * edges, over gradients that run to the screen's. The menus rise from the
 * buttons' row, whose place [anchors] keeps. While a panel [covered]s
 * them they aren't drawn, but stay in place, so focus goes back to the button
 * that opened it.
 */
@Composable
private fun Controls(
    title: String,
    state: PlayerState,
    play: PlayStarted?,
    speed: Float,
    hasEpisodes: Boolean,
    hasPrevious: Boolean,
    hasNext: Boolean,
    scrubTarget: Long?,
    scrubBar: FocusRequester,
    playButton: FocusRequester,
    tracksButton: FocusRequester,
    hasTracks: Boolean,
    preview: (Long) -> SpriteFrame?,
    onToggle: () -> Unit,
    onBack: () -> Unit,
    onForward: () -> Unit,
    onScrub: (direction: Int, repeatCount: Int) -> Unit,
    onCommitScrub: () -> Boolean,
    onPrevious: () -> Unit,
    onNext: () -> Unit,
    onMenu: (PlayerMenu) -> Unit,
    anchors: MenuAnchors,
    covered: Boolean,
) {
    val colors = Kivo.colors
    val live = state.live
    Box(Modifier.fillMaxSize().drawWithContent { if (!covered) drawContent() }) {
        Row(
            Modifier
                .fillMaxWidth()
                .background(Brush.verticalGradient(listOf(colors.playerScrim, Color.Transparent)))
                .padding(horizontal = SafeEdge, vertical = SafeTop),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(title, style = TvType.large, color = colors.onPlayer, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
        }
        Column(
            Modifier
                .align(Alignment.BottomCenter)
                .fillMaxWidth()
                .background(Brush.verticalGradient(listOf(Color.Transparent, colors.playerScrim)))
                .padding(start = SafeEdge, end = SafeEdge, top = 28.dp, bottom = SafeTop),
            verticalArrangement = Arrangement.spacedBy(7.dp),
        ) {
            // The time played and the time left either side of the bar, and under the time left when
            // the video ends at the speed it plays at, from the position.
            val duration = play?.durationMs?.toLong() ?: 0
            ScrubBar(
                positionMs = state.positionMs,
                target = scrubTarget,
                durationMs = duration,
                endsAt = if (duration > 0) endsAt(((duration - state.positionMs) / speed).toLong()) else null,
                chapters = play?.chapters.orEmpty(),
                live = live,
                preview = preview,
                modifier = Modifier.focusRequester(scrubBar).focusProperties { down = playButton },
                onScrub = onScrub,
                // Once the video has ended, OK on the bar doesn't start it over.
                onOk = { if (!onCommitScrub() && !state.ended) onToggle() },
            )
            // The transport in the middle; Audio & Subtitles and Playback Settings at the right.
            Box(Modifier.fillMaxWidth().onGloballyPositioned { anchors.rowTop = it.boundsInWindow().top }) {
                Row(
                    Modifier.align(Alignment.Center),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(7.dp),
                ) {
                    if (hasEpisodes) PlayerButton(KivoIcon.Previous, stringResource(R.string.previous_episode), onPrevious, enabled = hasPrevious)
                    PlayerButton(KivoIcon.Replay, stringResource(R.string.back_10), onBack, enabled = live)
                    PlayerButton(
                        if (state.paused) KivoIcon.Play else KivoIcon.Pause,
                        stringResource(if (state.paused) R.string.play else R.string.pause),
                        onToggle,
                        Modifier.focusRequester(playButton),
                        enabled = live,
                        size = 49.dp,
                    )
                    PlayerButton(KivoIcon.Forward, stringResource(R.string.forward_30), onForward, enabled = live)
                    if (hasEpisodes) PlayerButton(KivoIcon.Next, stringResource(R.string.next_episode), onNext, enabled = hasNext)
                }
                if (play != null) {
                    Row(Modifier.align(Alignment.CenterEnd), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(7.dp)) {
                        // Down from hidden controls lands on Audio & Subtitles, or on the gear without it.
                        if (hasTracks) {
                            PlayerButton(
                                KivoIcon.SpeechBubble,
                                stringResource(R.string.audio_and_subtitles),
                                { onMenu(PlayerMenu.Tracks) },
                                Modifier.focusRequester(tracksButton).onGloballyPositioned { anchors.tracksEnd = it.boundsInWindow().right },
                            )
                        }
                        PlayerButton(
                            KivoIcon.Settings,
                            stringResource(R.string.playback_settings),
                            { onMenu(PlayerMenu.Settings) },
                            (if (hasTracks) Modifier else Modifier.focusRequester(tracksButton)).onGloballyPositioned { anchors.settingsEnd = it.boundsInWindow().right },
                        )
                    }
                }
            }
        }
    }
}

/**
 * The scrub bar: chapter marks as ticks, and [endsAt] under the time left;
 * with focus, Left and Right go to [onScrub] and OK, as it's released, to
 * [onOk]; a [target] shows above the bar with its preview frame (once the
 * play's previews are ready), time and chapter.
 */
@Composable
private fun ScrubBar(
    positionMs: Long,
    target: Long?,
    durationMs: Long,
    endsAt: String?,
    chapters: List<Chapter>,
    live: Boolean,
    preview: (Long) -> SpriteFrame?,
    modifier: Modifier = Modifier,
    onScrub: (direction: Int, repeatCount: Int) -> Unit,
    onOk: () -> Unit,
) {
    val colors = Kivo.colors
    var focused by remember { mutableStateOf(false) }
    val shown = target ?: positionMs
    val fraction = if (durationMs > 0) (shown.toFloat() / durationMs).coerceIn(0f, 1f) else 0f
    // The times' room depends only on the duration, so the bar's length holds as they tick.
    val hours = if (durationMs >= HOUR_MS) TimeHoursWidth else 0.dp
    val playedWidth = PlayedWidth + hours
    val leftWidth = LeftWidth + hours
    Column {
        BoxWithConstraints(Modifier.fillMaxWidth().padding(start = playedWidth, end = leftWidth).height(28.dp)) {
            val scrubbing = target
            if (scrubbing != null) {
                val chapter = chapterAt(chapters, scrubbing)?.title
                val frame = preview(scrubbing)
                val density = LocalDensity.current
                // Above the bar at the target, kept on screen; floating, so the controls don't move.
                val frameWidth = with(density) { (frame?.width ?: 0).toDp() }
                val x = (maxWidth * fraction - frameWidth / 2).coerceIn(0.dp, (maxWidth - frameWidth).coerceAtLeast(0.dp))
                Column(
                    Modifier
                        .align(Alignment.BottomStart)
                        .wrapContentSize(Alignment.BottomStart, unbounded = true)
                        .offset(x = x),
                    verticalArrangement = Arrangement.spacedBy(Kivo.space(1)),
                ) {
                    if (frame != null) PreviewFrame(frame)
                    Text(
                        listOfNotNull(display.clock(scrubbing), chapter).joinToString(" · "),
                        style = TvType.smallStrong,
                        color = colors.onPlayer,
                    )
                }
            }
        }
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(display.clock(shown), style = TvType.body, color = colors.onPlayer, maxLines = 1, modifier = Modifier.width(playedWidth))
            Box(
                modifier
                    .weight(1f)
                    .height(21.dp)
                    .testTag("scrub")
                    .onFocusChanged { focused = it.isFocused }
                    .onKeyEvent { event ->
                        if (!live) return@onKeyEvent false
                        val down = event.type == KeyEventType.KeyDown
                        when (event.key.nativeKeyCode) {
                            KeyEvent.KEYCODE_DPAD_LEFT -> { if (down) onScrub(-1, event.nativeKeyEvent.repeatCount); true }
                            KeyEvent.KEYCODE_DPAD_RIGHT -> { if (down) onScrub(1, event.nativeKeyEvent.repeatCount); true }
                            in OkKeys -> { if (!down) onOk(); true }
                            else -> false
                        }
                    }
                    .focusable(enabled = live),
                contentAlignment = Alignment.CenterStart,
            ) {
                val height = if (focused) 8.dp else 4.dp
                Box(Modifier.fillMaxWidth().height(height).clip(RoundedCornerShape(KivoTokens.Radius.full)).background(colors.playerTrack))
                Box(Modifier.fillMaxWidth(fraction).height(height).clip(RoundedCornerShape(KivoTokens.Radius.full)).background(colors.progress))
                if (durationMs > 0) {
                    Box(Modifier.fillMaxWidth()) {
                        for (chapter in chapters.drop(1)) {
                            val at = chapter.startMs.toFloat() / durationMs
                            Box(Modifier.fillMaxWidth(at).height(height), contentAlignment = Alignment.CenterEnd) {
                                Box(Modifier.width(2.dp).height(height).background(colors.onPlayerMuted))
                            }
                        }
                    }
                    Box(Modifier.fillMaxWidth(fraction), contentAlignment = Alignment.CenterEnd) {
                        Box(
                            Modifier
                                .offset(x = 8.dp)
                                .size(if (focused) 16.dp else 12.dp)
                                .clip(RoundedCornerShape(KivoTokens.Radius.full))
                                .background(if (focused) colors.focus else colors.onPlayer),
                        )
                    }
                }
            }
            Text(
                "−" + display.clock(durationMs - shown),
                style = TvType.body,
                color = colors.onPlayer,
                textAlign = TextAlign.End,
                maxLines = 1,
                modifier = Modifier.width(leftWidth),
            )
        }
        if (endsAt != null) {
            Text(endsAt, style = TvType.small, color = colors.onPlayerMuted, maxLines = 1, modifier = Modifier.align(Alignment.End))
        }
    }
}

/** Room for the time played beside the scrub bar, with the gap to it: "59:59" at most. */
private val PlayedWidth = 40.5.dp

/** Room for the time left beside the scrub bar, with the gap to it: "−59:59" at most. */
private val LeftWidth = 48.5.dp

/** More room for each time when a video's an hour or longer: "1:12:03". */
private val TimeHoursWidth = 16.dp

private const val HOUR_MS = 3_600_000L

/** One preview frame, cut from its sprite sheet at the size it was made. */
@Composable
private fun PreviewFrame(frame: SpriteFrame) {
    val session = LocalSession.current
    val density = LocalDensity.current
    val painter = rememberAsyncImagePainter(
        ImageRequest.Builder(LocalContext.current).data(frame.sheetUrl).size(coil3.size.Size.ORIGINAL).build(),
        imageLoader = session.images,
    )
    val sheet = painter.intrinsicSize
    with(density) {
        Box(
            Modifier
                .size(frame.width.toDp(), frame.height.toDp())
                .clip(RoundedCornerShape(KivoTokens.Radius.sm))
                .background(Kivo.colors.player),
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

@Composable
private fun PlayerButton(icon: KivoIcon, label: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true, size: Dp = 35.dp) {
    val colors = Kivo.colors
    Surface(
        onClick = onClick,
        enabled = enabled,
        modifier = modifier.size(size).testTag(label),
        shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(if (size > 35.dp) KivoTokens.Radius.full else KivoTokens.Radius.md)),
        colors = ClickableSurfaceDefaults.colors(
            containerColor = Color.Transparent,
            contentColor = colors.onPlayer,
            focusedContainerColor = Lit.container,
            focusedContentColor = Lit.content,
            disabledContainerColor = Color.Transparent,
            disabledContentColor = colors.onPlayerMuted.copy(alpha = 0.5f),
        ),
        border = ClickableSurfaceDefaults.border(
            focusedDisabledBorder = focusRing(RoundedCornerShape(if (size > 35.dp) KivoTokens.Radius.full else KivoTokens.Radius.md)),
        ),
        scale = ClickableSurfaceDefaults.scale(focusedScale = KivoTokens.Tv.focusScale),
    ) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { KivoIconView(icon, size = size * 0.55f, contentDescription = label) }
    }
}

/** "Skip Intro", "Skip Recap" or "Skip Credits", bottom right; it takes focus as it appears over hidden controls. */
@Composable
private fun SkipButton(marker: SkipMarker, focus: FocusRequester, onSkip: () -> Unit) {
    KivoButton(
        stringResource(
            when (marker.kind) {
                MarkerKind.Intro -> R.string.skip_intro
                MarkerKind.Recap -> R.string.skip_recap
                MarkerKind.Credits -> R.string.skip_credits
            },
        ),
        onSkip,
        Modifier.focusRequester(focus).testTag("skip"),
        primary = true,
    )
}

/**
 * The next episode, counting down to play; waiting for Play now with the
 * user's autoplay off; or "Still watching?" after three without input.
 */
@Composable
private fun UpNext(model: PlayerViewModel, upNext: UpNext, play: FocusRequester) {
    val next = model.screen.next ?: return
    val colors = Kivo.colors
    val countsDown = upNext.phase == UpNextPhase.CountingDown
    val asks = upNext.phase == UpNextPhase.StillWatching
    Column(
        Modifier
            .width(KivoTokens.Layout.upNextWidth)
            .clip(RoundedCornerShape(KivoTokens.Radius.md))
            .background(colors.playerPanel)
            .padding(Kivo.space(3)),
        verticalArrangement = Arrangement.spacedBy(Kivo.space(2)),
    ) {
        Artwork(next.images.still ?: next.images.backdrop, next.title, KivoTokens.Aspect.landscape, KivoTokens.Layout.upNextWidth)
        Text(stringResource(if (asks) R.string.still_watching else R.string.up_next).uppercase(), style = TvType.smallStrong, color = colors.onPlayerMuted)
        Text(listOfNotNull(next.episode?.let(display::episodeCode), next.title).joinToString(" · "), style = TvType.bodyStrong, color = colors.onPlayer, maxLines = 1, overflow = TextOverflow.Ellipsis)
        if (countsDown) Text(stringResource(R.string.playing_in, upNext.secondsLeft), style = TvType.small, color = colors.onPlayerMuted)
        Row(horizontalArrangement = Arrangement.spacedBy(Kivo.space(2))) {
            KivoButton(stringResource(R.string.play_now), model.video::playNext, Modifier.focusRequester(play).testTag("playNow"), primary = true)
            KivoButton(
                stringResource(if (upNext.atCredits) R.string.watch_credits else R.string.hide),
                model.video::hideUpNext,
                Modifier.testTag("hideUpNext"),
            )
        }
    }
}

/** What a remote's focus last went by: whether the controls showed, the media was live, and the button over the picture. */
private data class RemoteFocus(val visible: Boolean, val live: Boolean, val overlay: FocusRequester?, val stillWatching: Boolean)

/** The [RemoteFocus] the last move went by, kept across moves without drawing anything when it changes. */
private class LastShown {
    var focus = RemoteFocus(visible = false, live = false, overlay = null, stillWatching = false)
}

/** A turning ring over the video while it loads, buffers or seeks. */
@Composable
private fun Spinner(modifier: Modifier = Modifier) {
    val turn = rememberInfiniteTransition(label = "spinner")
        .animateFloat(0f, 360f, infiniteRepeatable(tween(1_000, easing = LinearEasing)), label = "turn")
    val color = Kivo.colors.onPlayer
    Canvas(
        modifier
            .size(48.dp)
            .graphicsLayer { rotationZ = turn.value },
    ) {
        drawArc(color, startAngle = 0f, sweepAngle = 270f, useCenter = false, style = Stroke(4.dp.toPx(), cap = StrokeCap.Round))
    }
}

/**
 * What the player opens over the video: the menus of Audio & Subtitles and
 * Playback Settings and the Quality, Version and Speed lists its rows open,
 * or the panels of the subtitle search and Stream info.
 */
private enum class PlayerMenu { Tracks, SubtitleSearch, Settings, Quality, Version, Speed, Info }

/** The speeds a video plays at, normal among them. */
private val Speeds = listOf(0.5f, 0.75f, 1f, 1.25f, 1.5f, 1.75f, 2f)

/** Whether Audio & Subtitles has something for [play]: a track to choose, or the dialogue boost. */
private fun tracksPanelShows(model: PlayerViewModel, play: PlayStarted): Boolean =
    trackChoices(model, play).choosable || play.dialogueBoost != null

/** The play's version's tracks, the ones playing, and whether its subtitles can be searched for. */
private fun trackChoices(model: PlayerViewModel, play: PlayStarted): TrackChoices {
    val version = model.screen.work?.versions?.firstOrNull { it.id == play.versionId }
    val tracks = version?.tracks.orEmpty()
    val audio = tracks.filter { it.type == TrackType.Audio }
    return TrackChoices(
        audio = audio,
        subtitles = tracks.filter { it.type == TrackType.Subtitle },
        audioSelected = model.audio ?: audio.firstOrNull { it.default }?.id ?: audio.firstOrNull()?.id,
        subtitleSelected = model.subtitle,
        searchable = SubtitleSearch.offered(version),
    )
}

/**
 * What's open over the video, the last of [menus]: the subtitle search or
 * Stream info, in a panel, or else a menu raised over the controls from the
 * button that opened the first of them ([anchors]), whose contents each one
 * opened from it replaces in place. [onOpen] opens another, [onBack] goes
 * back to the one before it, and [onClose] closes them all, as choosing a
 * quality, version or speed does.
 */
@Composable
private fun Menus(
    model: PlayerViewModel,
    play: PlayStarted,
    menus: List<PlayerMenu>,
    anchors: MenuAnchors,
    onOpen: (PlayerMenu) -> Unit,
    onBack: () -> Unit,
    onClose: () -> Unit,
) {
    // The Playback Settings row whose list was open last, focused as the menu comes back to it.
    var lastRow by remember { mutableStateOf<PlayerMenu?>(null) }
    when (val menu = menus.lastOrNull()) {
        null -> Unit
        // A subtitle chosen here plays at once, and Audio & Subtitles shows it chosen.
        PlayerMenu.SubtitleSearch -> SubtitleSearchPanel(
            display = LocalSession.current.graph.display,
            search = { model.searchSubtitles() },
            download = {
                model.downloadSubtitle(it)
                onBack()
            },
            onDismiss = onBack,
        )
        PlayerMenu.Info -> StreamInfoPanel(model, play, onBack)
        else -> {
            val tracks = trackChoices(model, play)
            val boost = play.dialogueBoost?.enabled
            val quality = qualityChoices(model, play)
            val versions = versionChoices(model)
            val speeds = Speeds.map { Choice(it.toString(), speedLabel(it)) }
            val settings = settingsRows(model, play, quality)
            // Playback Settings keeps one width through its lists; Audio & Subtitles is as wide as its columns.
            val settingsWidth = menuWidth(
                settings to ChoiceMarker.None,
                quality.choices to ChoiceMarker.Tick,
                versions to ChoiceMarker.Tick,
                speeds to ChoiceMarker.Tick,
                padding = PlayerMenuPadding,
            )
            MenuPopup(
                onDismiss = onBack,
                alignment = Alignment.BottomEnd,
                margin = anchors.above(menus.first()),
                width = if (menu == PlayerMenu.Tracks) tracksMenuWidth(tracks.columns(boost)) else settingsWidth,
                maxHeight = PlayerMenuMaxHeight,
                padding = PlayerMenuPadding,
            ) {
                key(menu) {
                    when (menu) {
                        PlayerMenu.Tracks -> TrackColumns(
                            tracks = tracks,
                            // The player has no Automatic, so a choice is always a track or Off.
                            onAudio = { if (it != null && it != model.audio) model.chooseAudio(it) },
                            onSubtitle = { if (it != null && it != tracks.subtitleSelected) model.chooseSubtitle(it) },
                            menu = true,
                            dialogueBoost = boost,
                            onDialogueBoost = model::boostDialogue,
                            onSearch = { onOpen(PlayerMenu.SubtitleSearch) },
                        )
                        PlayerMenu.Settings -> ChoiceList(
                            choices = settings,
                            selected = lastRow?.name,
                            ticked = emptySet(),
                            marker = ChoiceMarker.None,
                            menu = true,
                            onSelect = { id ->
                                val row = PlayerMenu.valueOf(id)
                                lastRow = row
                                if (row == PlayerMenu.Info) {
                                    onClose()
                                    onOpen(PlayerMenu.Info)
                                } else {
                                    onOpen(row)
                                }
                            },
                        )
                        // A list a row opens heads itself with a row of the row's name that goes back to it.
                        PlayerMenu.Quality -> ChoiceList(quality.choices, quality.selected, menu = true, back = stringResource(R.string.quality), onBack = onBack, onSelect = { id ->
                            onClose()
                            if (id != quality.selected) model.chooseQuality(Quality.of(id))
                        })
                        PlayerMenu.Version -> ChoiceList(versions, play.versionId, menu = true, back = stringResource(R.string.version), onBack = onBack, onSelect = { id ->
                            onClose()
                            if (id != play.versionId) model.chooseVersion(id)
                        })
                        PlayerMenu.Speed -> ChoiceList(speeds, model.speed.toString(), menu = true, back = stringResource(R.string.speed), onBack = onBack, onSelect = { id ->
                            onClose()
                            model.chooseSpeed(id.toFloat())
                        })
                        else -> Unit
                    }
                }
            }
        }
    }
}

/** The player's menus' padding, and the tallest they stand. */
private val PlayerMenuPadding = 7.dp
private val PlayerMenuMaxHeight = 392.dp

/**
 * Where the player's menus rise from, in the window's pixels: the top of
 * the controls' row of buttons, and the right edge of each button that
 * opens a menu.
 */
@Stable
private class MenuAnchors {
    var rowTop by mutableFloatStateOf(0f)
    var tracksEnd by mutableFloatStateOf(0f)
    var settingsEnd by mutableFloatStateOf(0f)

    /** A menu's margin over the row, at the end of the button that opens [opener]. */
    @Composable
    fun above(opener: PlayerMenu): PaddingValues {
        val window = LocalWindowInfo.current.containerSize
        return with(LocalDensity.current) {
            menuAbove(rowTop.toDp(), (if (opener == PlayerMenu.Tracks) tracksEnd else settingsEnd).toDp(), DpSize(window.width.toDp(), window.height.toDp()))
        }
    }
}

/**
 * Playback Settings, the gear's menu: Audio & Subtitles (when it has
 * something), Quality, Version (with several), Speed and Stream info.
 * Quality, Version and Speed show what's playing and open their lists in
 * the menu's place; Audio & Subtitles opens its columns there, and Stream
 * info the panel in the menu's place.
 */
@Composable
private fun settingsRows(model: PlayerViewModel, play: PlayStarted, quality: QualityChoices): List<Choice> {
    val versions = model.screen.work?.versions.orEmpty()
    return buildList {
        if (tracksPanelShows(model, play)) add(Choice(PlayerMenu.Tracks.name, stringResource(R.string.audio_and_subtitles)))
        add(Choice(PlayerMenu.Quality.name, stringResource(R.string.quality), value = quality.choices.firstOrNull { it.id == quality.selected }?.label, opens = true))
        if (versions.size > 1) {
            add(Choice(PlayerMenu.Version.name, stringResource(R.string.version), value = versions.firstOrNull { it.id == play.versionId }?.name, opens = true))
        }
        add(Choice(PlayerMenu.Speed.name, stringResource(R.string.speed), value = speedLabel(model.speed), opens = true))
        add(Choice(PlayerMenu.Info.name, stringResource(R.string.stream_info)))
    }
}

/** Quality's choices for [play], and the one it plays at. */
private class QualityChoices(val choices: List<Choice>, val selected: String)

/** Auto, Original and the ladder's rungs below the file's own bitrate, those above the server's cap disabled. */
@Composable
private fun qualityChoices(model: PlayerViewModel, play: PlayStarted): QualityChoices {
    val sourceBps = model.screen.work?.versions?.firstOrNull { it.id == play.versionId }?.bitrateBps
    val cap = play.limitBps
    val limited = cap?.let { stringResource(R.string.limited_for_play, display.bitrate(it)) }
    val selected = Quality.of(play.quality).key
    val autoDetail = stringResource(R.string.auto_detail)
    val choices = model.screen.qualities.map { option ->
        when (val q = option.quality) {
            Quality.Auto -> Choice(q.key, display.quality(option, null), autoDetail)
            Quality.Original -> Choice(q.key, display.quality(option, sourceBps), limited, enabled = cap == null)
            is Quality.Limit -> Choice(
                q.key,
                display.quality(option, null),
                if (cap != null && q.maxBitrateBps > cap) limited else null,
                enabled = cap == null || q.maxBitrateBps <= cap,
            )
        }
    }
    return QualityChoices(choices, selected)
}

/** The work's versions, a missing one disabled, each with its video codec and size. */
@Composable
private fun versionChoices(model: PlayerViewModel): List<Choice> {
    val display = LocalSession.current.graph.display
    val missing = stringResource(R.string.version_missing)
    return model.screen.work?.versions.orEmpty().map { v ->
        val codec = v.tracks.firstOrNull { it.type == TrackType.Video }?.let { display.codec(it.codec) }
        Choice(
            v.id,
            if (v.available) v.name else missing.format(v.name),
            listOfNotNull(codec, display.size(v.sizeBytes)).joinToString(" · "),
            enabled = v.available,
        )
    }
}

/** "Normal" for 1, else the rate: "0.5×", "1.25×", "2×". */
@Composable
private fun speedLabel(speed: Float): String =
    if (speed == 1f) stringResource(R.string.speed_normal) else stringResource(R.string.speed_rate, if (speed % 1f == 0f) speed.toInt().toString() else speed.toString())

/** A capability the file failed, in words: "maxLevel 5.1; this device: 4.1". */
@Composable
private fun limitWords(): (CapabilityMismatch) -> String {
    val declared = stringResource(R.string.info_limit)
    val undeclared = stringResource(R.string.info_limit_undeclared)
    val channels = stringResource(R.string.info_limit_channels)
    val channelsUndeclared = stringResource(R.string.info_limit_channels_undeclared)
    return { limit ->
        // Channels read as words; other limits go by the name the server gives them.
        val device = limit.declared
        when {
            limit.field == "maxChannels" && device != null -> channels.format(limit.actual, device)
            limit.field == "maxChannels" -> channelsUndeclared.format(limit.actual)
            device != null -> declared.format(limit.field, limit.actual, device)
            else -> undeclared.format(limit.field, limit.actual)
        }
    }
}

@Composable
private fun StreamInfoPanel(model: PlayerViewModel, play: PlayStarted, onClose: () -> Unit) {
    val display = LocalSession.current.graph.display
    val version = model.screen.work?.versions?.firstOrNull { it.id == play.versionId }
    val subtitle = model.subtitle
    val words = StreamInfoWords(
        playback = stringResource(R.string.info_playback),
        container = stringResource(R.string.info_container),
        video = stringResource(R.string.info_video),
        audio = stringResource(R.string.audio),
        subtitles = stringResource(R.string.subtitles),
        quality = stringResource(R.string.quality),
        delivery = stringResource(R.string.info_delivery),
        bitrate = stringResource(R.string.info_bitrate),
        off = stringResource(R.string.off),
        hls = "HLS",
        file = stringResource(R.string.info_file),
        local = stringResource(R.string.info_local),
        remote = stringResource(R.string.info_remote),
        toHls = stringResource(R.string.info_to_hls).let { t -> { c: String -> t.format(c) } },
        cantPlayContainer = stringResource(R.string.info_cant_play_container).let { t -> { c: String -> t.format(c) } },
        originalUnder = stringResource(R.string.info_original_under).let { t -> { f: String, c: String -> t.format(f, c) } },
        limitedTo = stringResource(R.string.info_limited_to).let { t -> { c: String -> t.format(c) } },
        limit = limitWords(),
    )
    val rows = streamInfo(play, version, play.quality, subtitle, display, words)
    val why = stringResource(R.string.info_why)
    val keys = rememberPanelKeys()
    Dialog(onDismissRequest = onClose, properties = panelProperties()) {
        val focus = remember { FocusRequester() }
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.CenterEnd) {
            LazyColumn(
                Modifier
                    .onPreviewKeyEvent { keys.dispatch(it.nativeKeyEvent) }
                    .width(420.dp)
                    .fillMaxSize()
                    .background(LocalPanelSurface.current)
                    .padding(start = Kivo.space(6), top = SafeTop, end = SafeEdge, bottom = SafeTop)
                    .focusRequester(focus)
                    .focusable()
                    .testTag("stream-info"),
                verticalArrangement = Arrangement.spacedBy(Kivo.space(3)),
            ) {
                item { Text(stringResource(R.string.stream_info), style = TvType.heading, color = Kivo.colors.onPlayer) }
                items(rows) { row ->
                    Row {
                        Text(row.label, style = TvType.small, color = Kivo.colors.onPlayerMuted, modifier = Modifier.width(110.dp))
                        Column {
                            Text(row.value, style = TvType.smallStrong, color = Kivo.colors.onPlayer)
                            row.why?.let { Text(why.format(it), style = TvType.small, color = Kivo.colors.onPlayerMuted) }
                        }
                    }
                }
            }
        }
        LaunchedEffect(Unit) { focusWhenPlaced(focus) }
    }
}

/**
 * Playback stopped: what failed in a line, the server's reason in words,
 * then Try again, Play another version and Back; under them an admin also
 * sees the problem's code and reason.
 */
@Composable
private fun PlayerError(state: PlayerState, admin: Boolean, onRetry: () -> Unit, onPlayOther: (() -> Unit)?, onBack: () -> Unit) {
    val failure = state.failure ?: return
    val colors = Kivo.colors
    val display = LocalSession.current.graph.display
    val problem = failure as? PlayerFailure.Problem
    val limit = problem?.knownCode == ProblemCode.StreamLimit
    val title = when {
        failure is PlayerFailure.Ended -> stringResource(R.string.playback_stopped)
        limit -> stringResource(R.string.stream_limit_title)
        else -> stringResource(R.string.couldnt_play)
    }
    val sentence = display.failure(failure)
    val retry = remember { FocusRequester() }
    Box(Modifier.fillMaxSize().background(colors.playerScrim), contentAlignment = Alignment.Center) {
        Column(Modifier.widthIn(max = 640.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
            Text(title, style = TvType.heading, color = colors.onPlayer, textAlign = TextAlign.Center)
            Text(sentence, style = TvType.body, color = colors.onPlayerMuted, textAlign = TextAlign.Center)
            if (limit) {
                problem.plays?.forEach { other ->
                    Text(listOfNotNull(other.title, other.clientName).joinToString(" · "), style = TvType.small, color = colors.onPlayer)
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
                KivoButton(stringResource(R.string.try_again), onRetry, Modifier.focusRequester(retry).testTag("tryAgain"), primary = true)
                if (onPlayOther != null) KivoButton(stringResource(R.string.play_another_version), onPlayOther)
                KivoButton(stringResource(R.string.back), onBack)
            }
            problem?.takeIf { admin }?.let { Text(it.reason?.let { r -> "${it.code} ($r)" } ?: it.code, style = TvType.small, color = colors.onPlayerMuted) }
        }
    }
    LaunchedEffect(Unit) { focusWhenPlaced(retry) }
}
