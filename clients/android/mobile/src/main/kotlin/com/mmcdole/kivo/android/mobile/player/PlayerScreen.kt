package com.mmcdole.kivo.android.mobile.player

import android.graphics.Color as AndroidColor
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.LocalActivity
import androidx.annotation.OptIn
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.gestures.detectVerticalDragGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.displayCutout
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.boundsInRoot
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.media3.common.util.UnstableApi
import androidx.media3.ui.AspectRatioFrameLayout
import androidx.media3.ui.CaptionStyleCompat
import androidx.media3.ui.PlayerView
import androidx.media3.ui.SubtitleView
import androidx.compose.ui.graphics.toArgb
import com.mmcdole.kivo.android.mobile.theme.KivoTokens
import com.mmcdole.kivo.android.settings.ScreenSettings
import com.mmcdole.kivo.android.settings.SubtitleBackground
import com.mmcdole.kivo.android.settings.SubtitleSize
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.app.PlayerRoute
import com.mmcdole.kivo.android.mobile.cast.LocalCast
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.android.player.LoadedPlayer
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.core.display.pickerName
import com.mmcdole.kivo.core.display.quality
import com.mmcdole.kivo.core.display.qualityMenu
import com.mmcdole.kivo.core.display.size
import com.mmcdole.kivo.core.play.PlayerStatus
import com.mmcdole.kivo.core.player.Markers
import com.mmcdole.kivo.core.player.Subtitles
import com.mmcdole.kivo.core.settings.Quality
import io.github.peerless2012.ass.media.widget.AssSubtitleView
import kotlinx.coroutines.delay

/** The controls hide this long after the last touch while playing. */
private const val CONTROLS_HIDE_MS = 3_000L

/** A double tap's skips on one side add up while each comes within this long of the last. */
private const val BURST_MS = 900L

/**
 * The video player, full screen and landscape: the video and its
 * subtitles; a tap shows or hides the controls, which hide by themselves
 * while it plays, a double tap on the left or right third skips back 10 or
 * forward 30 seconds, and a swipe down or Back closes it. The controls are
 * Close, the title, Cast and Picture in picture along the top, and the scrub
 * bar, back 10, play or pause, forward 30, Audio & Subtitles and Playback
 * Settings along the bottom, whose popups stand above their buttons. Skip
 * Intro, Recap or Credits, up next for episodes, and the error with Try
 * again. Leaving the app shrinks it into picture in picture while it plays;
 * otherwise the play stops, and starts again where it was on return.
 */
@Composable
fun PlayerScreen(session: ServerSession, route: PlayerRoute, nav: MobileNav) {
    val model = viewModel { PlayerViewModel(session.session, route, { session.readyDownload(it)?.id }, PlayerViewModel.engineFor(session)) }
    val state by model.state.collectAsStateWithLifecycle()
    val playback = state.playback
    val live = playback.live
    val activity = LocalActivity.current as? ComponentActivity

    LifecycleEventEffect(Lifecycle.Event.ON_STOP) { model.stopOnExit() }
    LifecycleEventEffect(Lifecycle.Event.ON_START) { model.restore() }
    PlayerWindow(
        awake = when (playback.status) {
            PlayerStatus.Loading, PlayerStatus.Buffering, PlayerStatus.Seeking -> true
            PlayerStatus.Playing -> !playback.paused
            else -> false
        },
    )
    val video = state.version?.tracks?.firstOrNull { it.type == TrackType.Video }?.video
    val aspect = pictureInPictureAspect(video?.width, video?.height)
    val pictureInPicture = pictureInPictureOffered()
    if (pictureInPicture) PictureInPicture(live && !playback.paused, aspect, model::togglePlay, model::setPictureInPicture)

    // A TV connected while this plays: the TV carries on from here, and the remote takes the player's place.
    val cast = LocalCast.current
    if (cast != null) {
        LaunchedEffect(cast) {
            cast.connections.collect {
                val handoff = model.handOff() ?: return@collect
                if (cast.cast(handoff)) nav.replacePlayerWithRemote()
            }
        }
    }
    LaunchedEffect(Unit) {
        for (exit in model.exits) {
            when (exit) {
                PlayerExit.Back -> nav.back()
                is PlayerExit.Replace -> nav.replacePlayer(exit.route)
            }
        }
    }

    val menus = remember { MenuState() }
    val anchors = remember { MenuAnchors() }
    var scrubbing by remember { mutableStateOf(false) }
    var touches by remember { mutableIntStateOf(0) }
    val visible = state.overlay.controlsShown
    LaunchedEffect(menus.open != null) { model.setMenuOpen(menus.open != null) }
    LaunchedEffect(visible, live, playback.paused, menus.open, scrubbing, touches) {
        if (!visible || !live || playback.paused || menus.open != null || scrubbing) return@LaunchedEffect
        delay(CONTROLS_HIDE_MS)
        model.hideControls()
    }
    BackHandler { nav.back() }

    val failed = playback.status == PlayerStatus.Error && playback.failure != null
    Box(
        Modifier
            .fillMaxSize()
            .background(Color.Black)
            .onGloballyPositioned { anchors.origin = it.boundsInRoot() },
    ) {
        val screen by session.graph.screenSettings.values.collectAsStateWithLifecycle(ScreenSettings())
        Video(state.surface, screen)
        if (state.pictureInPicture) return@Box

        var burst by remember { mutableStateOf<Burst?>(null) }
        LaunchedEffect(burst) {
            if (burst != null) {
                delay(BURST_MS)
                burst = null
            }
        }
        VideoGestures(
            onTap = {
                touches++
                model.toggleControls()
            },
            onSkip = { forward ->
                if (live) {
                    val last = burst
                    burst = if (last != null && last.forward == forward) last.copy(count = last.count + 1) else Burst(forward, 1)
                    model.nudge(forward)
                }
            },
            onClose = nav::back,
        )
        burst?.let { b ->
            val seconds = b.count * ((if (b.forward) Markers.FORWARD_MS else Markers.BACK_MS) / 1000).toInt()
            SkipBurstLabel(b.forward, seconds, Modifier.align(if (b.forward) Alignment.CenterEnd else Alignment.CenterStart).padding(horizontal = space(16)))
        }

        if (playback.status == PlayerStatus.Loading || playback.status == PlayerStatus.Buffering || playback.status == PlayerStatus.Seeking) {
            Spinner(Modifier.align(Alignment.Center))
        }

        val inset = Modifier.windowInsetsPadding(WindowInsets.displayCutout)
        if (visible && !failed) {
            PlayerTopRow(
                state.work,
                onClose = nav::back,
                onPictureInPicture = if (pictureInPicture) ({ enterPictureInPicture(activity, aspect) }) else null,
                modifier = Modifier.align(Alignment.TopCenter).then(inset),
            )
            PlayerBottomBar(
                state,
                hasTracks = tracksOffered(state),
                anchors = anchors,
                onToggle = {
                    touches++
                    model.togglePlay()
                },
                onSkip = model::skip,
                onScrubbing = { held ->
                    scrubbing = held
                    model.setScrubbing(held)
                },
                onSeek = model::seek,
                onMenu = menus::open,
                modifier = Modifier.align(Alignment.BottomCenter).then(inset),
            )
        }

        // Above the controls' bar while they show, else near the bottom.
        val next = state.next
        val marker = state.overlay.skipMarker
        Column(
            Modifier
                .align(Alignment.BottomEnd)
                .then(inset)
                .padding(end = space(4), bottom = if (visible) 132.dp else space(6)),
            horizontalAlignment = Alignment.End,
            verticalArrangement = Arrangement.spacedBy(space(3)),
        ) {
            if (next != null && state.overlay.upNext.shows && !failed) UpNextCard(next, state.overlay.upNext, model::playNext, model::hideUpNext)
            if (marker != null && !failed) SkipButton(marker, model::skipMarker)
        }

        if (failed) PlayerErrorPanel(playback, model::retry, nav::back)

        if (playback.play != null && !failed) {
            PlayerPopup(
                menus,
                anchor = menus.open?.let(anchors::of) ?: androidx.compose.ui.geometry.Rect.Zero,
                width = if (menus.open == PlayerMenu.Tracks) TracksWidth else SettingsWidth,
                pageOf = { menu, sub -> menuPage(state, menu, sub) },
                onChoose = { menu, sub, id -> choose(model, state, menus, menu, sub, id) },
            )
        }
    }
}

/** The popups' widths, each kept as its lists replace its rows. */
private val TracksWidth = 280.dp
private val SettingsWidth = 260.dp

/** A run of double-tap skips on one side, which the label totals. */
private data class Burst(val forward: Boolean, val count: Int)

/** The taps, double taps and swipe down on the video itself. */
@Composable
private fun VideoGestures(onTap: () -> Unit, onSkip: (forward: Boolean) -> Unit, onClose: () -> Unit) {
    Box(
        Modifier
            .fillMaxSize()
            .testTag("video")
            .pointerInput(Unit) {
                detectTapGestures(
                    onTap = { onTap() },
                    onDoubleTap = { at ->
                        val third = size.width / 3f
                        when {
                            at.x < third -> onSkip(false)
                            at.x > third * 2 -> onSkip(true)
                            else -> onTap()
                        }
                    },
                )
            }
            .pointerInput(Unit) {
                val far = 96.dp.toPx()
                var pulled = 0f
                detectVerticalDragGestures(onDragStart = { pulled = 0f }, onDragEnd = { if (pulled > far) onClose() }) { _, dy -> pulled += dy }
            },
    )
}

/**
 * The video and its subtitles: Media3's view, plain subtitles in the size
 * and background Settings › Subtitles chose on this phone, and libass over
 * it for styled ones, which keep their own look.
 */
@OptIn(UnstableApi::class)
@Composable
private fun Video(player: LoadedPlayer?, screen: ScreenSettings) {
    val box = KivoTokens.dark.subtitleBox.toArgb()
    AndroidView(
        modifier = Modifier.fillMaxSize(),
        factory = { context ->
            PlayerView(context).apply {
                useController = false
                resizeMode = AspectRatioFrameLayout.RESIZE_MODE_FIT
                setShutterBackgroundColor(AndroidColor.BLACK)
            }
        },
        update = { view ->
            if (view.player !== player?.player) {
                view.player = player?.player
                val subtitles = view.subtitleView ?: return@AndroidView
                for (i in subtitles.childCount - 1 downTo 0) {
                    if (subtitles.getChildAt(i) is AssSubtitleView) subtitles.removeViewAt(i)
                }
                player?.let { subtitles.addView(AssSubtitleView(view.context, it.ass)) }
            }
            view.subtitleView?.apply {
                setFractionalTextSize(
                    when (screen.subtitleSize) {
                        SubtitleSize.Small -> 0.045f
                        SubtitleSize.Large -> 0.07f
                        SubtitleSize.Medium -> SubtitleView.DEFAULT_TEXT_SIZE_FRACTION
                    },
                )
                setStyle(
                    CaptionStyleCompat(
                        AndroidColor.WHITE,
                        if (screen.subtitleBackground == SubtitleBackground.Box) box else AndroidColor.TRANSPARENT,
                        AndroidColor.TRANSPARENT,
                        if (screen.subtitleBackground == SubtitleBackground.Shadow) CaptionStyleCompat.EDGE_TYPE_DROP_SHADOW else CaptionStyleCompat.EDGE_TYPE_NONE,
                        AndroidColor.BLACK,
                        null,
                    ),
                )
            }
        },
    )
}

/** Whether Audio & Subtitles has something to choose: more than one audio track, or a subtitle track. */
private fun tracksOffered(state: PlayerUiState): Boolean {
    val tracks = state.version?.tracks.orEmpty()
    return tracks.count { it.type == TrackType.Audio } > 1 || tracks.any { it.type == TrackType.Subtitle }
}

/** The speeds a video plays at, normal among them. */
private val Speeds = listOf(0.5f, 0.75f, 1f, 1.25f, 1.5f, 1.75f, 2f)

/**
 * What a popup shows. Audio & Subtitles: Audio and Subtitles, each with
 * what plays and opening its tracks, Subtitles with Off first. Playback
 * Settings: Quality (Auto, Original, then the ladder's rungs below the
 * file's bitrate, those above a limit on the play disabled), Version where
 * there are several, Speed, and the Dialogue boost switch where the play
 * offers it.
 */
@Composable
private fun menuPage(state: PlayerUiState, menu: PlayerMenu, sub: SubMenu?): MenuPage {
    val play = state.playback.play ?: return MenuPage(emptyList())
    val version = state.version
    val tracks = version?.tracks.orEmpty()
    val audio = tracks.filter { it.type == TrackType.Audio }
    val subtitles = tracks.filter { it.type == TrackType.Subtitle }
    val audioId = state.playback.audioTrackId ?: version?.audioTrackId ?: audio.firstOrNull()?.id
    val subtitleId = state.playback.subtitleTrackId
    val off = stringResource(R.string.mobile_off)

    val sourceBps = version?.bitrateBps
    val cap = play.limitBps
    val selectedQuality = Quality.of(play.quality).key
    val qualities = qualityMenu(state.ladder.filter { sourceBps == null || it.bitrateBps < sourceBps }).map { option ->
        val q = option.quality
        MenuItem(
            q.key,
            display.quality(option, if (q == Quality.Original) sourceBps else null),
            ticked = q.key == selectedQuality,
            enabled = when (q) {
                Quality.Auto -> true
                Quality.Original -> cap == null
                is Quality.Limit -> cap == null || q.maxBitrateBps <= cap
            },
        )
    }
    val missing = stringResource(R.string.mobile_version_missing)
    val versions = state.work?.versions.orEmpty().map { v ->
        MenuItem(v.id, if (v.available) v.name else missing.format(v.name), display.size(v.sizeBytes), ticked = v.id == play.versionId, enabled = v.available)
    }
    val speeds = Speeds.map { MenuItem(it.toString(), speedLabel(it), ticked = it == state.speed) }

    return when (sub) {
        SubMenu.Audio -> MenuPage(audio.map { MenuItem(it.id, it.pickerName.name, it.pickerName.note, ticked = it.id == audioId) }, stringResource(R.string.mobile_audio))
        SubMenu.Subtitles -> MenuPage(
            listOf(MenuItem(Subtitles.OFF, off, ticked = subtitleId == Subtitles.OFF)) +
                subtitles.map { MenuItem(it.id, it.pickerName.name, it.pickerName.note, ticked = it.id == subtitleId) },
            stringResource(R.string.mobile_subtitles),
        )
        SubMenu.Quality -> MenuPage(qualities, stringResource(R.string.mobile_quality))
        SubMenu.Version -> MenuPage(versions, stringResource(R.string.mobile_version))
        SubMenu.Speed -> MenuPage(speeds, stringResource(R.string.mobile_speed))
        null -> when (menu) {
            PlayerMenu.Tracks -> MenuPage(
                buildList {
                    if (audio.size > 1) add(MenuItem(SubMenu.Audio.name, stringResource(R.string.mobile_audio), audio.firstOrNull { it.id == audioId }?.pickerName?.name, opens = true))
                    if (subtitles.isNotEmpty()) {
                        val shown = if (subtitleId == Subtitles.OFF) off else subtitles.firstOrNull { it.id == subtitleId }?.pickerName?.name
                        add(MenuItem(SubMenu.Subtitles.name, stringResource(R.string.mobile_subtitles), shown, opens = true))
                    }
                },
            )
            PlayerMenu.Settings -> MenuPage(
                buildList {
                    add(MenuItem(SubMenu.Quality.name, stringResource(R.string.mobile_quality), qualities.firstOrNull { it.ticked }?.label, opens = true))
                    if (versions.size > 1) add(MenuItem(SubMenu.Version.name, stringResource(R.string.mobile_version), version?.name, opens = true))
                    add(MenuItem(SubMenu.Speed.name, stringResource(R.string.mobile_speed), speedLabel(state.speed), opens = true))
                    play.dialogueBoost?.let { add(MenuItem(BOOST, stringResource(R.string.mobile_dialogue_boost), switch = it.enabled)) }
                },
            )
        }
    }
}

/** A popup's choice: a track, quality, version or speed plays at once and closes it; the dialogue boost switches and it stays. */
private fun choose(model: PlayerViewModel, state: PlayerUiState, menus: MenuState, menu: PlayerMenu, sub: SubMenu?, id: String) {
    val play = state.playback.play ?: return
    when (sub) {
        SubMenu.Audio -> if (id != state.playback.audioTrackId) model.chooseAudio(id)
        SubMenu.Subtitles -> if (id != state.playback.subtitleTrackId) model.chooseSubtitle(id)
        SubMenu.Quality -> if (id != Quality.of(play.quality).key) model.chooseQuality(Quality.of(id))
        SubMenu.Version -> if (id != play.versionId) model.chooseVersion(id)
        SubMenu.Speed -> model.chooseSpeed(id.toFloat())
        null -> {
            if (menu == PlayerMenu.Settings && id == BOOST) model.boostDialogue(play.dialogueBoost?.enabled != true)
            return
        }
    }
    menus.close()
}

/** The dialogue boost's row. */
private const val BOOST = "dialogueBoost"

/** "Normal" for 1, else the rate: "0.5×", "1.25×", "2×". */
@Composable
private fun speedLabel(speed: Float): String =
    if (speed == 1f) stringResource(R.string.mobile_speed_normal) else stringResource(R.string.mobile_speed_rate, if (speed % 1f == 0f) speed.toInt().toString() else speed.toString())
