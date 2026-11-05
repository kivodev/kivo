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
import com.mmcdole.kivo.core.data.SubtitleSearch
import com.mmcdole.kivo.core.player.Markers
import com.mmcdole.kivo.core.player.SpriteFrame
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
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
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
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
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.Artwork
import com.mmcdole.kivo.android.ui.labelledTitle
import com.mmcdole.kivo.android.ui.Choice
import com.mmcdole.kivo.android.ui.ChoiceMarker
import com.mmcdole.kivo.android.ui.ChoicePanel
import com.mmcdole.kivo.android.ui.SubtitleSearchPanel
import com.mmcdole.kivo.android.ui.TrackChoices
import com.mmcdole.kivo.android.ui.TracksPanel
import com.mmcdole.kivo.android.ui.choosable
import com.mmcdole.kivo.android.ui.endsAt
import com.mmcdole.kivo.core.display.bitrate
import com.mmcdole.kivo.core.display.clock
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.display.quality
import com.mmcdole.kivo.core.display.qualityMenu
import com.mmcdole.kivo.core.display.size
import com.mmcdole.kivo.core.display.versionName
import com.mmcdole.kivo.android.ui.KivoButton
import com.mmcdole.kivo.android.ui.KivoIcon
import com.mmcdole.kivo.android.ui.KeyAction
import com.mmcdole.kivo.android.ui.KivoIconView
import com.mmcdole.kivo.android.ui.LocalKeyDispatcher
import com.mmcdole.kivo.android.ui.LocalSession
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.OkKeys
import com.mmcdole.kivo.android.ui.panelProperties
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.rememberKeyDispatcher
import com.mmcdole.kivo.android.ui.rememberPanelKeys
import com.mmcdole.kivo.android.ui.focusRing
import com.mmcdole.kivo.android.ui.focusWhenPlaced
import com.mmcdole.kivo.android.ui.display
import com.mmcdole.kivo.api.models.Chapter
import com.mmcdole.kivo.api.models.CapabilityMismatch
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.core.player.MarkerKind
import com.mmcdole.kivo.core.player.SkipMarker
import com.mmcdole.kivo.core.player.UpNext
import com.mmcdole.kivo.core.player.UpNextPhase
import com.mmcdole.kivo.core.play.PlayerFailure
import com.mmcdole.kivo.core.play.failure
import com.mmcdole.kivo.core.play.PlayerState
import com.mmcdole.kivo.core.play.PlayerStatus
import com.mmcdole.kivo.core.settings.Quality
import com.mmcdole.kivo.android.settings.SubtitleBackground
import com.mmcdole.kivo.android.settings.SubtitleSize
import kotlinx.coroutines.delay

/** A scrub seeks to its target after this long without another step. */
private const val SCRUB_COMMIT_MS = 1_000L

/** The controls hide this long after the last key while playing. */
private const val CONTROLS_HIDE_MS = 3_000L

/**
 * The player: the video with its subtitles, controls that show on
 * any input and hide after 3 seconds while playing, the remote map,
 * Audio & Subtitles, Playback Settings and the stream info, Skip Intro, Recap and Credits, up next
 * for episodes, and the error with its recovery actions.
 */
@Composable
fun PlayerScreen(session: ServerSession, args: Screen.Player, navigator: Navigator) {
    val model = viewModel { PlayerViewModel(session, args) }
    val state by model.controller.state.collectAsStateWithLifecycle()
    val user by session.session.user.collectAsStateWithLifecycle()
    val current = state.play
    val work = model.work
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
    val frameRate = version?.tracks?.firstOrNull { it.type == "video" }?.video?.frameRate
    val matching = model.screen?.matchesFrameRate == true
    DisposableEffect(activity, frameRate, matching) {
        val matcher = activity?.let(::FrameRateMatcher)
        if (matching && frameRate != null) matcher?.match(frameRate.toFloat())
        onDispose { matcher?.release() }
    }

    LaunchedEffect(Unit) {
        for (exit in model.exits) {
            when (exit) {
                PlayerExit.Back -> navigator.pop()
                is PlayerExit.Replace -> navigator.replace(exit.screen)
                is PlayerExit.Series -> navigator.returnTo(Screen.Work(exit.seriesId))
            }
        }
    }

    // The controls, Skip and Up next show by the core's rules, from the inputs given it; the controls hide by the wait below.
    val video = model.video
    val ui by video.ui.collectAsStateWithLifecycle()
    val visible = ui.controlsShown
    // The panels open over the video, the one showing last: Back goes to the one before it.
    var menus by remember { mutableStateOf(emptyList<PlayerMenu>()) }
    val menu = menus.lastOrNull()
    val root = remember { FocusRequester() }
    val scrubBar = remember { FocusRequester() }
    val playButton = remember { FocusRequester() }
    val tracksButton = remember { FocusRequester() }
    // Where focus goes as the controls show, when the key that showed them chose; else the scrub bar.
    var revealFocus by remember { mutableStateOf<FocusRequester?>(null) }

    val showUpNext = model.next != null && ui.upNext.shows
    val stillWatching = showUpNext && ui.upNext.phase == UpNextPhase.StillWatching
    val marker = ui.skipMarker
    // A panel open holds the controls up and up next's countdown.
    LaunchedEffect(menu != null) { video.setMenuOpen(open = menu != null) }

    fun reveal(focus: FocusRequester? = null) {
        if (!video.ui.value.controlsShown) revealFocus = focus
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
    // The keys pressed so far: each starts the controls' wait to hide again.
    var keysPressed by remember { mutableIntStateOf(0) }
    // Shown while playing with no panel open, the controls hide once the keys stop.
    LaunchedEffect(visible, live, state.paused, menu != null, keysPressed) {
        if (!visible || !live || state.paused || menu != null) return@LaunchedEffect
        delay(CONTROLS_HIDE_MS)
        hide()
    }

    fun perform(command: PlayerCommand, repeatCount: Int) {
        val c = model.controller
        when (command) {
            PlayerCommand.TogglePlay -> { reveal(); c.togglePlay() }
            PlayerCommand.Play -> { reveal(); c.resume() }
            PlayerCommand.Pause -> { reveal(); c.pause() }
            PlayerCommand.SkipBack -> skipBy(-Markers.BACK_MS)
            PlayerCommand.SkipForward -> skipBy(Markers.FORWARD_MS)
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
        // Any input means someone is watching: up next may play on again.
        video.userActivity()
        keysPressed++
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
     * Opens [opened] from its button or over the panel whose row opened it:
     * a panel's window takes focus, so the press that opened it ends there.
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
        Video(model.player, model.screen?.subtitleSize, model.screen?.subtitleBackground)

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
                preview = { at -> model.previews?.let { (url, manifest) -> SpriteFrame.atOrNull(manifest, url, at) } },
                onToggle = { model.controller.togglePlay() },
                onBack = { model.controller.skip(-Markers.BACK_MS) },
                onForward = { model.controller.skip(Markers.FORWARD_MS) },
                onScrub = ::scrub,
                onCommitScrub = ::commitScrub,
                onPrevious = model::playPrevious,
                onNext = video::playNext,
                onMenu = ::openMenu,
            )
        }

        Column(Modifier.align(Alignment.BottomEnd).padding(end = 48.dp, bottom = 150.dp), horizontalAlignment = Alignment.End) {
            if (showUpNext) {
                UpNext(model, ui.upNext, playNow)
                Spacer(Modifier.height(Kivo.space(3)))
            }
            if (marker != null) SkipButton(marker, skipButton, video::skipMarker)
        }

        if (failed) {
            val other = work?.versions?.firstOrNull { it.available && it.id != (current?.versionId ?: args.versionId) }
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
        // The menus' windows hand their keys to the player's dispatcher.
        CompositionLocalProvider(LocalKeyDispatcher provides keys) {
            Menus(model, current, menu, onOpen = ::openMenu, onBack = { menus = menus.dropLast(1) }, onClose = { menus = emptyList() })
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
) {
    val colors = Kivo.colors
    val live = state.live
    Box(Modifier.fillMaxSize()) {
        Row(
            Modifier
                .fillMaxWidth()
                .background(Brush.verticalGradient(listOf(colors.playerScrim, Color.Transparent)))
                .padding(horizontal = 48.dp, vertical = 27.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(title, style = TvType.heading, color = colors.onPlayer, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
            // When the video ends at the speed it plays at, from the position.
            val durationMs = play?.durationMs?.toLong() ?: 0
            if (durationMs > 0) {
                Text(
                    endsAt(((durationMs - state.positionMs) / speed).toLong()),
                    style = TvType.body,
                    color = colors.onPlayerMuted,
                    modifier = Modifier.padding(start = Kivo.space(4)),
                )
            }
        }
        Column(
            Modifier
                .align(Alignment.BottomCenter)
                .fillMaxWidth()
                .background(Brush.verticalGradient(listOf(Color.Transparent, colors.playerScrim)))
                .padding(start = 48.dp, end = 48.dp, top = 64.dp, bottom = 27.dp),
            verticalArrangement = Arrangement.spacedBy(Kivo.space(3)),
        ) {
            // The time played and the time left either side of the bar.
            val duration = play?.durationMs?.toLong() ?: 0
            ScrubBar(
                positionMs = state.positionMs,
                target = scrubTarget,
                durationMs = duration,
                chapters = play?.chapters.orEmpty(),
                live = live,
                preview = preview,
                modifier = Modifier.focusRequester(scrubBar).focusProperties { down = playButton },
                onScrub = onScrub,
                // Once the video has ended, OK on the bar doesn't start it over.
                onOk = { if (!onCommitScrub() && !state.ended) onToggle() },
            )
            // The transport in the middle; Audio & Subtitles and Playback Settings at the right.
            Box(Modifier.fillMaxWidth()) {
                Row(
                    Modifier.align(Alignment.Center),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(Kivo.space(2)),
                ) {
                    if (hasEpisodes) PlayerButton(KivoIcon.Previous, stringResource(R.string.previous_episode), onPrevious, enabled = hasPrevious)
                    PlayerButton(KivoIcon.Replay, stringResource(R.string.back_10), onBack, enabled = live)
                    PlayerButton(
                        if (state.paused) KivoIcon.Play else KivoIcon.Pause,
                        stringResource(if (state.paused) R.string.play else R.string.pause),
                        onToggle,
                        Modifier.focusRequester(playButton),
                        enabled = live,
                        size = 60.dp,
                    )
                    PlayerButton(KivoIcon.Forward, stringResource(R.string.forward_30), onForward, enabled = live)
                    if (hasEpisodes) PlayerButton(KivoIcon.Next, stringResource(R.string.next_episode), onNext, enabled = hasNext)
                }
                if (play != null) {
                    Row(Modifier.align(Alignment.CenterEnd), verticalAlignment = Alignment.CenterVertically) {
                        // Down from hidden controls lands on Audio & Subtitles, or on the gear without it.
                        if (hasTracks) {
                            PlayerTextButton(
                                stringResource(R.string.audio_and_subtitles),
                                { onMenu(PlayerMenu.Tracks) },
                                Modifier.focusRequester(tracksButton),
                                icon = KivoIcon.SpeechBubble,
                            )
                        }
                        PlayerButton(
                            KivoIcon.Settings,
                            stringResource(R.string.playback_settings),
                            { onMenu(PlayerMenu.Settings) },
                            if (hasTracks) Modifier else Modifier.focusRequester(tracksButton),
                        )
                    }
                }
            }
        }
    }
}

/**
 * The scrub bar: chapter marks as ticks; with focus, Left and Right go to
 * [onScrub] and OK, as it's released, to [onOk]; a [target] shows above the bar with its
 * preview frame (once the play's previews are ready), time and chapter.
 */
@Composable
private fun ScrubBar(
    positionMs: Long,
    target: Long?,
    durationMs: Long,
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
    Column {
        BoxWithConstraints(Modifier.fillMaxWidth().padding(horizontal = TimeWidth).height(28.dp)) {
            val scrubbing = target
            if (scrubbing != null) {
                val chapter = Markers.chapterAt(chapters, scrubbing)?.title
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
            Text(display.clock(shown), style = TvType.body, color = colors.onPlayer, modifier = Modifier.width(TimeWidth))
            Box(
                modifier
                    .weight(1f)
                    .height(20.dp)
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
                modifier = Modifier.width(TimeWidth),
            )
        }
    }
}

/** Room for a time beside the scrub bar, "1:12:03" at most. */
private val TimeWidth = 80.dp

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
private fun PlayerButton(icon: KivoIcon, label: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true, size: Dp = 48.dp) {
    val colors = Kivo.colors
    Surface(
        onClick = onClick,
        enabled = enabled,
        modifier = modifier.size(size).testTag(label),
        shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(if (size > 48.dp) KivoTokens.Radius.full else KivoTokens.Radius.md)),
        colors = ClickableSurfaceDefaults.colors(
            containerColor = Color.Transparent,
            contentColor = colors.onPlayer,
            focusedContainerColor = colors.playerTrack,
            focusedContentColor = colors.onPlayer,
            disabledContainerColor = Color.Transparent,
            disabledContentColor = colors.onPlayerMuted.copy(alpha = 0.5f),
        ),
        border = ClickableSurfaceDefaults.border(
            focusedBorder = focusRing(RoundedCornerShape(if (size > 48.dp) KivoTokens.Radius.full else KivoTokens.Radius.md)),
        ),
        scale = ClickableSurfaceDefaults.scale(focusedScale = KivoTokens.Tv.focusScale),
    ) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { KivoIconView(icon, size = size * 0.55f, contentDescription = label) }
    }
}

/** A control named in words, readable across the room, with an [icon] before them where it has one. */
@Composable
private fun PlayerTextButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, icon: KivoIcon? = null) {
    val colors = Kivo.colors
    Surface(
        onClick = onClick,
        modifier = modifier.height(48.dp).testTag(text),
        shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(KivoTokens.Radius.md)),
        colors = ClickableSurfaceDefaults.colors(
            containerColor = Color.Transparent,
            contentColor = colors.onPlayer,
            focusedContainerColor = colors.playerTrack,
            focusedContentColor = colors.onPlayer,
        ),
        border = ClickableSurfaceDefaults.border(focusedBorder = focusRing()),
        scale = ClickableSurfaceDefaults.scale(focusedScale = KivoTokens.Tv.focusScale),
    ) {
        Row(
            Modifier.height(48.dp).padding(horizontal = Kivo.space(3)),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(Kivo.space(2)),
        ) {
            icon?.let { KivoIconView(it, size = 24.dp) }
            Text(text, style = TvType.bodyStrong)
        }
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
    val next = model.next ?: return
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
 * A panel the player opens over the video: Audio & Subtitles and the
 * subtitle search its row opens, Playback Settings and the Quality, Version
 * and Speed lists its rows open, or the stream info overlay.
 */
private enum class PlayerMenu { Tracks, SubtitleSearch, Settings, Quality, Version, Speed, Info }

/** The speeds a video plays at, normal among them. */
private val Speeds = listOf(0.5f, 0.75f, 1f, 1.25f, 1.5f, 1.75f, 2f)

/** Whether Audio & Subtitles has something for [play]: a track to choose, or the dialogue boost. */
private fun tracksPanelShows(model: PlayerViewModel, play: PlayStarted): Boolean =
    trackChoices(model, play).choosable || play.dialogueBoost != null

/** The play's version's tracks, the ones playing, and whether its subtitles can be searched for. */
private fun trackChoices(model: PlayerViewModel, play: PlayStarted): TrackChoices {
    val version = model.version(play)
    val tracks = version?.tracks.orEmpty()
    val audio = tracks.filter { it.type == "audio" }
    return TrackChoices(
        audio = audio,
        subtitles = tracks.filter { it.type == "subtitle" },
        audioSelected = model.audio ?: audio.firstOrNull { it.default }?.id ?: audio.firstOrNull()?.id,
        subtitleSelected = model.subtitle,
        searchable = SubtitleSearch.offered(version),
    )
}

/**
 * The panel open over the video, [menu]: [onOpen] opens another over it,
 * [onBack] goes back to the one before it, and [onClose] closes them all,
 * as choosing a quality, version or speed does.
 */
@Composable
private fun Menus(
    model: PlayerViewModel,
    play: PlayStarted,
    menu: PlayerMenu?,
    onOpen: (PlayerMenu) -> Unit,
    onBack: () -> Unit,
    onClose: () -> Unit,
) {
    // The Playback Settings row whose list was open last, focused as the panel comes back.
    var lastRow by remember { mutableStateOf<PlayerMenu?>(null) }
    when (menu) {
        PlayerMenu.Tracks -> {
            val tracks = trackChoices(model, play)
            val display = LocalSession.current.graph.display
            TracksPanel(
                tracks = tracks,
                display = display,
                // The player has no Automatic, so a choice is always a track or Off.
                onAudio = { if (it != null && it != model.audio) model.chooseAudio(it) },
                onSubtitle = { if (it != null && it != tracks.subtitleSelected) model.chooseSubtitle(it) },
                onDismiss = onBack,
                dialogueBoost = play.dialogueBoost?.enabled,
                onDialogueBoost = model::boostDialogue,
                onSearch = { onOpen(PlayerMenu.SubtitleSearch) },
            )
        }
        // A subtitle chosen here plays at once, and Audio & Subtitles shows it chosen.
        PlayerMenu.SubtitleSearch -> SubtitleSearchPanel(
            display = LocalSession.current.graph.display,
            search = { model.searchSubtitles(play) },
            download = {
                model.downloadSubtitle(play, it)
                onBack()
            },
            onDismiss = onBack,
        )
        PlayerMenu.Settings -> SettingsMenu(model, play, lastRow, onBack) { row ->
            lastRow = row
            if (row == PlayerMenu.Info) {
                onClose()
                onOpen(PlayerMenu.Info)
            } else {
                onOpen(row)
            }
        }
        PlayerMenu.Quality -> QualityMenu(model, play, onBack, onClose)
        PlayerMenu.Version -> VersionMenu(model, play, onBack, onClose)
        PlayerMenu.Speed -> SpeedMenu(model, onBack, onClose)
        PlayerMenu.Info -> StreamInfoPanel(model, play, onBack)
        null -> Unit
    }
}

/**
 * Playback Settings, the gear's panel: Audio & Subtitles (when it has
 * something), Quality, Version (with several), Speed and Stream info, each
 * with its current value; a row opens its list, Audio & Subtitles that
 * panel, and Stream info the overlay in its place. [focused] is the row to
 * focus first, the one whose list was open.
 */
@Composable
private fun SettingsMenu(model: PlayerViewModel, play: PlayStarted, focused: PlayerMenu?, onDismiss: () -> Unit, onRow: (PlayerMenu) -> Unit) {
    val quality = qualityChoices(model, play)
    val versions = model.work?.versions.orEmpty()
    val display = LocalSession.current.graph.display
    val rows = buildList {
        if (tracksPanelShows(model, play)) add(Choice(PlayerMenu.Tracks.name, stringResource(R.string.audio_and_subtitles)))
        add(Choice(PlayerMenu.Quality.name, stringResource(R.string.quality), quality.choices.firstOrNull { it.id == quality.selected }?.label))
        if (versions.size > 1) {
            add(Choice(PlayerMenu.Version.name, stringResource(R.string.version), versions.firstOrNull { it.id == play.versionId }?.let(display::versionName)))
        }
        add(Choice(PlayerMenu.Speed.name, stringResource(R.string.speed), speedLabel(model.speed)))
        add(Choice(PlayerMenu.Info.name, stringResource(R.string.stream_info)))
    }
    ChoicePanel(
        title = stringResource(R.string.playback_settings),
        choices = rows,
        selected = focused?.name,
        ticked = emptySet(),
        marker = ChoiceMarker.None,
        onSelect = { onRow(PlayerMenu.valueOf(it)) },
        onDismiss = onDismiss,
    )
}

/** Quality's choices for [play], and the one it plays at. */
private class QualityChoices(val choices: List<Choice>, val selected: String)

/** Auto, Original and the ladder's rungs below the file's own bitrate, those above the server's cap disabled. */
@Composable
private fun qualityChoices(model: PlayerViewModel, play: PlayStarted): QualityChoices {
    val sourceBps = model.work?.versions?.firstOrNull { it.id == play.versionId }?.bitrateBps
    val cap = play.limitBps
    val limited = cap?.let { stringResource(R.string.limited_for_play, display.bitrate(it)) }
    val selected = Quality.of(play.quality).key
    // Only rungs below the file's own bitrate change anything.
    val rungs = model.ladder.filter { sourceBps == null || it.bitrateBps < sourceBps }
    val autoDetail = stringResource(R.string.auto_detail)
    val choices = qualityMenu(rungs).map { option ->
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

@Composable
private fun QualityMenu(model: PlayerViewModel, play: PlayStarted, onBack: () -> Unit, onClose: () -> Unit) {
    val quality = qualityChoices(model, play)
    ChoicePanel(
        title = stringResource(R.string.quality),
        choices = quality.choices,
        selected = quality.selected,
        onSelect = { id ->
            onClose()
            if (id != quality.selected) model.chooseQuality(Quality.of(id))
        },
        onDismiss = onBack,
    )
}

/** The work's versions, a missing one disabled, each with its video codec and size. */
@Composable
private fun VersionMenu(model: PlayerViewModel, play: PlayStarted, onBack: () -> Unit, onClose: () -> Unit) {
    val display = LocalSession.current.graph.display
    val missing = stringResource(R.string.version_missing)
    val choices = model.work?.versions.orEmpty().map { v ->
        val codec = v.tracks.firstOrNull { it.type == "video" }?.let { display.codec(it.codec) }
        Choice(
            v.id,
            display.versionName(v).let { if (v.available) it else missing.format(it) },
            listOfNotNull(codec, display.size(v.sizeBytes)).joinToString(" · "),
            enabled = v.available,
        )
    }
    ChoicePanel(
        title = stringResource(R.string.version),
        choices = choices,
        selected = play.versionId,
        onSelect = { id ->
            onClose()
            if (id != play.versionId) model.chooseVersion(id)
        },
        onDismiss = onBack,
    )
}

/** The speeds, from 0.5× to 2×, the one playing ticked. */
@Composable
private fun SpeedMenu(model: PlayerViewModel, onBack: () -> Unit, onClose: () -> Unit) {
    ChoicePanel(
        title = stringResource(R.string.speed),
        choices = Speeds.map { Choice(it.toString(), speedLabel(it)) },
        selected = model.speed.toString(),
        onSelect = { id ->
            onClose()
            model.chooseSpeed(id.toFloat())
        },
        onDismiss = onBack,
    )
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
    return { limit -> limit.declared?.let { declared.format(limit.field, limit.actual, it) } ?: undeclared.format(limit.field, limit.actual) }
}

@Composable
private fun StreamInfoPanel(model: PlayerViewModel, play: PlayStarted, onClose: () -> Unit) {
    val display = LocalSession.current.graph.display
    val version = model.work?.versions?.firstOrNull { it.id == play.versionId }
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
                    .background(Kivo.colors.playerPanel)
                    .padding(Kivo.space(6))
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
    val limit = problem?.code == "streamLimit"
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
