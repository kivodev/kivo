package com.mmcdole.kivo.android.detail

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.view.KeyEvent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.focusGroup
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.layout.layout
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.tv.material3.ClickableSurfaceDefaults
import androidx.tv.material3.Surface
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.main.RailWidth
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.Backdrop
import com.mmcdole.kivo.android.ui.CardGap
import com.mmcdole.kivo.android.ui.CardRow
import com.mmcdole.kivo.android.ui.CardShape
import com.mmcdole.kivo.android.ui.RowsHoldPlace
import com.mmcdole.kivo.android.ui.bringsTopIntoView
import com.mmcdole.kivo.android.ui.labelledTitle
import com.mmcdole.kivo.android.ui.Choice
import com.mmcdole.kivo.android.ui.ChoicePanel
import com.mmcdole.kivo.android.ui.AutomaticTracks
import com.mmcdole.kivo.android.ui.SubtitleSearchPanel
import com.mmcdole.kivo.android.ui.TrackChoices
import com.mmcdole.kivo.android.ui.TracksPanel
import com.mmcdole.kivo.android.ui.choosable
import com.mmcdole.kivo.android.ui.Chip
import com.mmcdole.kivo.android.ui.ControlLabel
import com.mmcdole.kivo.android.ui.CollectionCard
import com.mmcdole.kivo.android.ui.collectionCount
import com.mmcdole.kivo.android.ui.Failed
import com.mmcdole.kivo.android.ui.InlineFailed
import com.mmcdole.kivo.android.ui.FocusRail
import com.mmcdole.kivo.android.ui.FocusMemory
import com.mmcdole.kivo.android.ui.IconButton
import com.mmcdole.kivo.android.ui.endsAt
import com.mmcdole.kivo.android.ui.KivoButton
import com.mmcdole.kivo.android.ui.KivoIcon
import com.mmcdole.kivo.android.ui.KivoIconView
import com.mmcdole.kivo.android.ui.LandscapeWidth
import com.mmcdole.kivo.android.ui.Loader
import com.mmcdole.kivo.android.ui.SkeletonBlock
import com.mmcdole.kivo.android.ui.SkeletonCard
import com.mmcdole.kivo.android.ui.SkeletonGate
import com.mmcdole.kivo.android.ui.SkeletonHero
import com.mmcdole.kivo.android.ui.SkeletonLine
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.PersonCard
import com.mmcdole.kivo.android.ui.PosterWidth
import com.mmcdole.kivo.android.ui.RowTitle
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.SafeTop
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.TrailerCard
import com.mmcdole.kivo.android.ui.ValueRow
import com.mmcdole.kivo.android.ui.WorkCard
import com.mmcdole.kivo.android.ui.LocalSession
import com.mmcdole.kivo.android.ui.Overview
import com.mmcdole.kivo.android.ui.display
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.EpisodeCardRow
import com.mmcdole.kivo.android.ui.EpisodeCardRowSkeleton
import com.mmcdole.kivo.android.ui.FocusBleed
import com.mmcdole.kivo.android.ui.Lit
import com.mmcdole.kivo.android.ui.LocalCardLight
import com.mmcdole.kivo.android.ui.rememberCardLight
import com.mmcdole.kivo.android.ui.rememberOneMovePerFrame
import com.mmcdole.kivo.android.ui.rowEntry
import com.mmcdole.kivo.android.ui.rowPlace
import com.mmcdole.kivo.android.ui.raisedOnFocus
import com.mmcdole.kivo.android.ui.formatRating
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.CollectionSummary
import com.mmcdole.kivo.api.models.ImageRef
import com.mmcdole.kivo.api.models.Track
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.data.CreditRole
import com.mmcdole.kivo.core.data.Seasons
import com.mmcdole.kivo.core.data.SubtitleSearch
import com.mmcdole.kivo.core.data.knownRole
import com.mmcdole.kivo.core.data.localTrailers
import com.mmcdole.kivo.core.data.otherExtras
import com.mmcdole.kivo.core.data.resumeAt
import com.mmcdole.kivo.core.data.selectedVersion
import com.mmcdole.kivo.core.data.summary
import com.mmcdole.kivo.core.data.withTrack
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.display.clock
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.display.runtime
import com.mmcdole.kivo.core.display.size
import com.mmcdole.kivo.core.display.timeLeft
import com.mmcdole.kivo.core.player.Subtitles

/**
 * The detail page's choices, carried into playback. A null version is the
 * work's default one ([selectedVersion]); a null track leaves it to the
 * server.
 */
private data class Choices(val versionId: String? = null, val audioTrackId: String? = null, val subtitleTrackId: String? = null)

/**
 * A work's page: a movie or video, a series (a season opens its series on
 * it) or an episode. What's known of the work shows at once,
 * its artwork, title and details line, and the rest fills in as it's read.
 */
@Composable
fun DetailScreen(session: ServerSession, id: String, navigator: Navigator, episode: EpisodeInfo? = null) {
    val model = viewModel(key = id) { DetailViewModel(session, id) }
    Shown(model.work, model.seed, session) { page ->
        val season = page.head.season
        when {
            model.seasonOf != null -> SeriesDetail(model, page, id, null, navigator)
            page.head.kind == WorkKind.Series -> SeriesDetail(model, page, null, episode, navigator)
            page.head.kind == WorkKind.Season && season != null ->
                Shown(model.other(season.seriesId), model.seriesSeed(page.head), session) { SeriesDetail(model, it, page.head.id, null, navigator) }
            page.head.kind == WorkKind.Episode -> EpisodeDetail(model, page, navigator)
            else -> MovieDetail(model, page, navigator)
        }
    }
}

/**
 * A page as far as it's known: [head], what shows at once, and [work] once
 * it's read, or the [problem] reading it, which [retry] reads again.
 */
private class Page(val head: WorkSummary, val work: WorkDetail?, val problem: RequestProblem?, val retry: () -> Unit)

/**
 * A page's read: at once from [seed] when something's known of the work,
 * else its skeleton after a moment; a failure fills the screen only when
 * nothing's known. A page read stays while a later read fails.
 */
@Composable
private fun Shown(loader: Loader<WorkDetail>, seed: WorkSummary?, session: ServerSession, content: @Composable (Page) -> Unit) {
    val state by loader.state.collectAsStateWithLifecycle()
    var read by remember(loader) { mutableStateOf<WorkDetail?>(null) }
    state.getOrNull()?.let { read = it }
    val work = read
    val head = remember(work) { work?.summary() } ?: seed
    val problem = state.problemOrNull()
    if (head == null && problem != null) {
        Failed(problem, session.session.baseUrl, loader::retry, session.graph::changeServer)
        return
    }
    SkeletonGate(head, { SkeletonHero(Modifier.padding(start = 24.dp, top = 120.dp)) }) {
        content(Page(it, work, problem.takeIf { work == null }, loader::retry))
    }
}

/** Under the hero while the page is read: its overview's lines to come, or why it couldn't be read. */
@Composable
private fun Pending(page: Page) {
    val problem = page.problem
    when {
        page.work != null -> Unit
        problem != null -> InlineFailed(problem, LocalSession.current.session.baseUrl, page.retry)
        else -> Column {
            SkeletonLine(560.dp, TvType.body)
            SkeletonLine(520.dp, TvType.body)
            SkeletonLine(300.dp, TvType.body)
        }
    }
}

/** The D-pad keys that move focus. */
private val MoveKeys = setOf(KeyEvent.KEYCODE_DPAD_UP, KeyEvent.KEYCODE_DPAD_DOWN, KeyEvent.KEYCODE_DPAD_LEFT, KeyEvent.KEYCODE_DPAD_RIGHT)

/** How long focus stays before a card's light comes back on. */

/**
 * The page's frame: the backdrop across the whole screen, under the rail,
 * darkened at the left so text reads on any artwork and fading into the
 * background. A held key moves focus at most once a frame, and a focused
 * card's light goes out as focus moves on before resting, as on Home.
 */
@Composable
private fun DetailFrame(backdrop: ImageRef?, content: @Composable () -> Unit) {
    val colors = Kivo.colors
    val moves = remember { mutableIntStateOf(0) }
    val dropMove = rememberOneMovePerFrame()
    val light = rememberCardLight { moves.intValue }
    Box(
        Modifier
            .fillMaxSize()
            .onPreviewKeyEvent { event ->
                if (event.type != KeyEventType.KeyDown || event.key.nativeKeyCode !in MoveKeys) return@onPreviewKeyEvent false
                moves.intValue++
                dropMove()
            },
    ) {
        if (backdrop != null) {
            val layers = remember(colors) {
                listOf(
                    Brush.horizontalGradient(
                        0f to colors.bg.copy(alpha = 0.95f),
                        0.5f to colors.bg.copy(alpha = 0.92f),
                        0.72f to colors.bg.copy(alpha = 0.75f),
                        1f to colors.scrimSoft,
                    ),
                    // Solid by the foot of the screen, where a season's captions are.
                    Brush.verticalGradient(0f to colors.bg.copy(alpha = 0f), 0.85f to colors.bg),
                )
            }
            Backdrop(backdrop, BackdropWidth, layers, Modifier.underRail())
        }
        CompositionLocalProvider(LocalCardLight provides light, content = content)
    }
}

/** A backdrop's width: the whole TV canvas, so it's decoded for the pixels it covers. */
private val BackdropWidth = 960.dp

/**
 * Lays this out from the screen's left edge rather than from where the screen
 * starts, beside the rail, which is drawn over it. Nothing above a page clips,
 * and this only places it, so it costs no layer.
 */
private fun Modifier.underRail(): Modifier = layout { measurable, constraints ->
    val rail = RailWidth.roundToPx()
    val width = constraints.maxWidth + rail
    val placeable = measurable.measure(constraints.copy(minWidth = width, maxWidth = width))
    layout(constraints.maxWidth, placeable.height) { placeable.place(-rail, 0) }
}

/**
 * A detail page's list, its title [top] from the top of the screen. What
 * takes focus comes to rest [pivot] from there as the rows move under it, far
 * enough down to show a row's title above its cards; focus in the hero keeps
 * the whole hero in view.
 */
@Composable
private fun DetailList(top: Dp, pivot: Dp = top, bottom: Dp = SafeEdge, content: LazyListScope.() -> Unit) {
    RowsHoldPlace(pivot) {
        LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(start = 24.dp, top = top, bottom = bottom), content = content)
    }
}

// A hero's measures, from the Samsung TV app's at a dp to two of its pixels.

/** Where a movie's or series' title starts. */
private val HeroTop = 72.dp

/** The gap between a movie's or series' hero's parts, and half of it, which an episode's are, and under the buttons. */
private val HeroGap = 14.dp
private val HeroGapTight = 7.dp

/** What a series' list leaves below its last row, so its episodes can come up to the screen's top. */
private val SeriesRoomBelow = 300.dp

/** A hero's buttons. */
private val ButtonHeight = 35.dp
private val ButtonPadding = PaddingValues(horizontal = 14.dp)

/** A line of a hero's details. */
private val MetaHeight = 16.dp

/** A line of a description. */
private val OverviewLine = 20.dp

/**
 * How wide a movie's or series' title and description go, and an episode's,
 * which has the screen's width up to its right margin, where a season's fourth
 * card ends.
 */
private val HeroWidth = 518.dp
private val WideWidth = 808.dp

/** An episode's description: its text, and its block, which leaves room for More. */
private val EpisodeTextWidth = 532.dp
private val EpisodeBlockWidth = 616.dp

/** A season's row: its pills, and its cards under its heading. */
private val PillHeight = 31.5.dp
private val CardsUnderHeading = 10.5.dp

/** A row's title's gap to its room for focus, and the gaps above a row. */
private val RowTitleGap = 7.5.dp
private val RowGap = 19.dp

/** A hero's title, in display type. */
@Composable
private fun HeroTitle(text: String, maxLines: Int = 2, width: Dp = HeroWidth) {
    Text(text, style = TvType.display, color = Kivo.colors.text, maxLines = maxLines, overflow = TextOverflow.Ellipsis, modifier = Modifier.widthIn(max = width))
}

/** A line of a hero's details, which holds its place when it's empty. */
@Composable
private fun HeroLine(text: String) {
    Text(text, style = TvType.small, color = Kivo.colors.textMuted, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.height(MetaHeight))
}

/**
 * A hero's description, holding its [lines] whatever its length; while the
 * page is read, its lines to come or why it couldn't be read.
 */
@Composable
private fun OverviewSlot(
    hero: HeroLevels,
    page: Page,
    title: String,
    text: String?,
    lines: Int = 3,
    width: Dp = HeroWidth,
    textWidth: Dp = 470.dp,
) {
    Box(Modifier.heightIn(min = OverviewLine * lines)) {
        if (!text.isNullOrBlank()) {
            Overview(
                title,
                text,
                Modifier.focusKey(hero.memory, HeroLevels.OVERVIEW),
                lines = lines,
                width = width,
                textWidth = textWidth,
                onClamped = { hero.overviewStops = it },
            )
        } else {
            Pending(page)
        }
    }
}

/** The title and line of a collection's hero. */
@Composable
private fun HeroText(heading: String, meta: String) {
    val colors = Kivo.colors
    Column(Modifier.widthIn(max = 600.dp), verticalArrangement = Arrangement.spacedBy(Kivo.space(2))) {
        Text(heading, style = TvType.title, color = colors.text, maxLines = 2, overflow = TextOverflow.Ellipsis)
        if (meta.isNotEmpty()) Text(meta, style = TvType.body, color = colors.textMuted)
    }
}

/** The details line: year, runtime, community rating, content rating and, once read, genres. */
private fun metaLine(head: WorkSummary, work: WorkDetail?, display: Display): String = listOfNotNull(
    head.year?.toString(),
    head.durationMs?.let { display.runtime(it.toLong()) },
    head.communityRating?.let { "★ ${formatRating(it)}" },
    head.contentRating,
    work?.genres?.take(2)?.joinToString(", ")?.ifEmpty { null },
).joinToString(" · ")

@Composable
private fun MovieDetail(model: DetailViewModel, page: Page, navigator: Navigator) {
    val memory = rememberFocusMemory()
    val hero = rememberHeroLevels(memory)
    hero.levels = false
    var choices by rememberChoices()
    val head = page.head
    val work = page.work
    DetailFrame(head.images.backdrop) {
        DetailList(top = HeroTop) {
            item {
                Column(Modifier.bringsTopIntoView()) {
                    HeroTitle(labelledTitle(head.title, head.movie))
                    Spacer(Modifier.height(HeroGap))
                    HeroLine(metaLine(head, work, display))
                    Spacer(Modifier.height(HeroGap))
                    ActionRow(model, head, work, choices, hero, navigator)
                    Spacer(Modifier.height(HeroGapTight))
                    TimeLeft(head.userState?.positionMs, head.durationMs)
                    Spacer(Modifier.height(HeroGap))
                    // Two lines of pickers' room, as the Samsung TV app's wrap, so the description holds its place.
                    Box(Modifier.heightIn(min = 66.5.dp)) {
                        if (work != null) Pickers(work, choices, hero) { choices = it }
                    }
                    Spacer(Modifier.height(HeroGap))
                    OverviewSlot(hero, page, head.title, work?.overview)
                }
            }
            if (work != null) extraRows(work, memory, navigator, firstTop = 24.dp)
        }
    }
    LaunchedEffect(Unit) { memory.restore(if (playable(head, work, choices)) "play" else "myList") }
}

/** Whether the work, in the version selected, can play; before its versions are read, whether its card says it can. */
private fun playable(head: WorkSummary, work: WorkDetail?, choices: Choices) = if (work == null) {
    head.available
} else {
    work.available && work.selectedVersion(choices.versionId)?.available == true
}

/**
 * Play or Resume with the time, and beside Resume a round Play from
 * beginning; then [chips], My List, Played and the ⋯ menu; "This file is
 * missing" in Play's place when no version can play. They work from the
 * card before the page is read.
 */
@Composable
private fun ActionRow(
    model: DetailViewModel,
    work: WorkSummary,
    read: WorkDetail?,
    choices: Choices,
    hero: HeroLevels,
    navigator: Navigator,
    chips: (@Composable () -> Unit)? = null,
) {
    val played = work.userState?.played == true
    val myListId by model.myListId.collectAsStateWithLifecycle()
    val onMyList = read.onList(myListId)
    val playable = playable(work, read, choices)
    val position = if (read != null) read.resumeAt(choices.versionId) else work.userState?.positionMs
    fun play(fromStart: Boolean) = navigator.push(
        Screen.Player(work.id, read?.selectedVersion(choices.versionId)?.id ?: choices.versionId, choices.audioTrackId, choices.subtitleTrackId, fromStart),
    )
    Row(Modifier.heroButtons(hero), horizontalArrangement = Arrangement.spacedBy(HeroGapTight), verticalAlignment = Alignment.CenterVertically) {
        if (!playable) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                KivoIconView(KivoIcon.Alert, tint = Kivo.colors.warning)
                Text(stringResource(R.string.file_missing), style = TvType.bodyStrong, color = Kivo.colors.warning, modifier = Modifier.padding(start = Kivo.space(2)))
            }
        } else {
            PlayButton(if (position != null) stringResource(R.string.resume_at, display.clock(position.toLong())) else stringResource(R.string.play), hero) { play(fromStart = false) }
            if (position != null) RestartButton(hero) { play(fromStart = true) }
        }
        chips?.invoke()
        HeroIconButton(
            if (onMyList) KivoIcon.ListCheck else KivoIcon.ListPlus,
            stringResource(if (onMyList) R.string.remove_from_my_list else R.string.add_to_my_list),
            hero,
            "myList",
            onMyList,
        ) { model.setOnMyList(work.id, !onMyList) }
        HeroIconButton(
            KivoIcon.Check,
            stringResource(if (played) R.string.mark_unplayed else R.string.mark_played),
            hero,
            "played",
            played,
        ) { model.setPlayed(work.id, !played) }
        MoreButton(work, hero, navigator)
    }
}

/** A hero's Play or Resume. */
@Composable
private fun PlayButton(text: String, hero: HeroLevels, onClick: () -> Unit) {
    OfferButton(hero, "play")
    KivoButton(
        text = text,
        onClick = onClick,
        icon = KivoIcon.Play,
        primary = true,
        modifier = Modifier.heroButton(hero, "play").height(ButtonHeight).testTag("play"),
        contentPadding = ButtonPadding,
    )
}

/** One of a hero's small buttons, the button [key] of its row. */
@Composable
private fun HeroIconButton(icon: KivoIcon, label: String, hero: HeroLevels, key: String, on: Boolean = false, onClick: () -> Unit) {
    OfferButton(hero, key)
    IconButton(icon, label, onClick, Modifier.heroButton(hero, key), on = on, size = ButtonHeight, corners = KivoTokens.Radius.md)
}

/** Play from beginning, beside Resume. */
@Composable
private fun RestartButton(hero: HeroLevels, onClick: () -> Unit) {
    OfferButton(hero, "restart")
    IconButton(KivoIcon.Replay, stringResource(R.string.play_from_beginning), onClick, Modifier.heroButton(hero, "restart").testTag("restart"), size = ButtonHeight, corners = KivoTokens.Radius.md)
}

/** The hero's ⋯: Go to series for an episode, and Go to library. */
@Composable
private fun MoreButton(work: WorkSummary, hero: HeroLevels, navigator: Navigator) {
    var open by remember { mutableStateOf(false) }
    val seriesId = work.episode?.seriesId
    val libraryId = work.libraryId
    val choices = buildList {
        if (seriesId != null) add(Choice("series", stringResource(R.string.go_to_series)))
        if (libraryId != null) add(Choice("library", stringResource(R.string.go_to_library)))
    }
    if (choices.isEmpty()) return
    HeroIconButton(KivoIcon.More, stringResource(R.string.more_actions), hero, "more") { open = true }
    if (open) {
        ChoicePanel(
            title = work.title,
            choices = choices,
            selected = null,
            onSelect = {
                open = false
                when (it) {
                    "series" -> work.episode?.let { navigator.push(Screen.Work(it.seriesId, it)) }
                    "library" -> libraryId?.let { id -> navigator.open(Screen.Library(id)) }
                }
            },
            onDismiss = { open = false },
        )
    }
}

/** "1 hr 34 min left · Ends at 9:42 PM" under the buttons once a work is started, else when it would end. */
@Composable
private fun TimeLeft(positionMs: Int?, durationMs: Int?) {
    HeroLine(if (durationMs == null) "" else timeLeftLine(positionMs, durationMs))
}

/** The time left once a work is started and when it would end, each second as the clock moves. */
@Composable
private fun timeLeftLine(positionMs: Int?, durationMs: Int): String {
    val leftMs = (durationMs - (positionMs ?: 0)).toLong()
    return listOfNotNull(display.timeLeft(leftMs).takeIf { positionMs != null }, endsAt(leftMs)).joinToString(" · ")
}

@Composable
private fun rememberChoices() = rememberSaveable(stateSaver = androidx.compose.runtime.saveable.listSaver(
    save = { listOf(it.versionId, it.audioTrackId, it.subtitleTrackId) },
    restore = { Choices(it[0], it[1], it[2]) },
)) { mutableStateOf(Choices()) }

/** The picker whose choices are open, or the subtitle search Audio & Subtitles opens. */
private enum class Picker { Version, Tracks, SubtitleSearch }

/**
 * The version picker, and Audio & Subtitles, which opens the player's
 * Audio & Subtitles panel ([TracksPanel]) with Automatic heading each
 * column, named after the track the server says a play starts with. A
 * version change resets the track choices, whose IDs belong to one file.
 * Its subtitle search chooses the subtitle it downloads for the next play,
 * which shows among the tracks until the work is read again with it. As
 * [chips] they're buttons of the hero's row, "Audio: English 5.1" and
 * "Subtitles: Off", saying what plays where Automatic is chosen, as on an
 * episode's page.
 */
@Composable
private fun Pickers(read: WorkDetail, choices: Choices, hero: HeroLevels, chips: Boolean = false, onChange: (Choices) -> Unit) {
    var downloaded by remember(read) { mutableStateOf(emptyList<Pair<String, Track>>()) }
    val work = downloaded.fold(read) { w, (versionId, track) -> w.withTrack(versionId, track) }
    val version = work.selectedVersion(choices.versionId) ?: return
    val tracks = TrackChoices(
        audio = version.tracks.filter { it.type == TrackType.Audio },
        subtitles = version.tracks.filter { it.type == TrackType.Subtitle },
        audioSelected = choices.audioTrackId,
        subtitleSelected = choices.subtitleTrackId,
        searchable = SubtitleSearch.offered(version),
    )
    val automatic = AutomaticTracks(version.audioTrackId, version.subtitleTrackId ?: Subtitles.OFF)
    var open by remember { mutableStateOf<Picker?>(null) }
    val offLabel = stringResource(R.string.off)
    val missingLabel = stringResource(R.string.version_missing)
    val display = LocalSession.current.graph.display

    // What plays: the track chosen, else the one Automatic plays.
    val audio = choices.audioTrackId ?: automatic.audio
    val subtitle = choices.subtitleTrackId ?: automatic.subtitle
    val audioValue = tracks.audio.firstOrNull { it.id == audio }?.name.takeIf { tracks.audio.size > 1 }
    val subtitleValue = (if (subtitle == Subtitles.OFF) offLabel else tracks.subtitles.firstOrNull { it.id == subtitle }?.name)
        .takeIf { tracks.subtitles.isNotEmpty() || tracks.searchable }
    if (chips) {
        if (work.versions.size > 1) PickerChip(stringResource(R.string.version), version.name, hero, "version") { open = Picker.Version }
        if (tracks.choosable) {
            audioValue?.let { PickerChip(stringResource(R.string.audio), it, hero, "audio") { open = Picker.Tracks } }
            subtitleValue?.let { PickerChip(stringResource(R.string.subtitles), it, hero, "subtitles") { open = Picker.Tracks } }
        }
    } else {
        // Each picker's text lines up with the title above; its focus background reaches past it.
        Row(Modifier.offset(x = -Kivo.space(3)), horizontalArrangement = Arrangement.spacedBy(Kivo.space(2))) {
            if (work.versions.size > 1) {
                ValueRow(stringResource(R.string.version), version.name, { open = Picker.Version }, Modifier.widthIn(max = 220.dp))
            }
            if (tracks.choosable) {
                val value = listOfNotNull(audioValue, subtitleValue).joinToString(" / ")
                ValueRow(stringResource(R.string.audio_and_subtitles), value, { open = Picker.Tracks }, Modifier.widthIn(max = 420.dp).testTag("tracks"))
            }
        }
    }
    when (open) {
        Picker.Version -> ChoicePanel(
            title = stringResource(R.string.version),
            choices = work.versions.map { v ->
                val codec = v.tracks.firstOrNull { it.type == TrackType.Video }?.let { display.codec(it.codec) }
                val name = v.name
                Choice(v.id, if (v.available) name else missingLabel.format(name), listOfNotNull(codec, display.size(v.sizeBytes)).joinToString(" · "))
            },
            selected = version.id,
            onSelect = {
                open = null
                onChange(Choices(versionId = it))
            },
            onDismiss = { open = null },
        )
        Picker.Tracks -> TracksPanel(
            tracks = tracks,
            automatic = automatic,
            onAudio = { onChange(choices.copy(audioTrackId = it)) },
            onSubtitle = { onChange(choices.copy(subtitleTrackId = it)) },
            onDismiss = { open = null },
            onSearch = { open = Picker.SubtitleSearch },
        )
        Picker.SubtitleSearch -> {
            val session = LocalSession.current
            SubtitleSearchPanel(
                display = display,
                search = { session.session.playback.subtitleResults(version) },
                download = { result ->
                    val track = session.session.playback.addSubtitle(version, result.id)
                    downloaded = downloaded + (version.id to track)
                    onChange(choices.copy(subtitleTrackId = track.id))
                    open = Picker.Tracks
                },
                onDismiss = { open = Picker.Tracks },
            )
        }
        null -> Unit
    }
}

/** A picker among a hero's buttons. */
@Composable
private fun PickerChip(label: String, value: String, hero: HeroLevels, key: String, onClick: () -> Unit) {
    OfferButton(hero, key)
    KivoButton(
        text = stringResource(R.string.picker_value, label, value),
        onClick = onClick,
        modifier = Modifier.heroButton(hero, key).height(ButtonHeight).widthIn(max = 160.dp),
        contentPadding = ButtonPadding,
    )
}

/**
 * Trailers, cast, extras and collections under the hero, the first [firstTop] under it; rows with nothing in them are
 * left out. The Trailers row holds the trailer files in the work's folder, then a movie's trailers on YouTube.
 */
private fun LazyListScope.extraRows(work: WorkDetail, memory: FocusMemory, navigator: Navigator, firstTop: Dp = RowGap) {
    var top = firstTop
    fun nextTop() = top.also { top = RowGap }
    val local = work.localTrailers()
    val online = work.movie?.trailers.orEmpty()
    if (local.isNotEmpty() || online.isNotEmpty()) {
        extraRow(R.string.trailers, nextTop()) {
            items(local, key = { it.id }) { w ->
                WorkCard(
                    w,
                    CardShape.Landscape,
                    LandscapeWidth,
                    { navigator.push(Screen.Work(w.id)) },
                    Modifier.focusKey(memory, "row/${w.id}"),
                    mark = w.durationMs?.let { display.clock(it.toLong()) },
                    caption = stringResource(R.string.in_your_library),
                )
            }
            items(online, key = { it.id }) { trailer ->
                val context = LocalContext.current
                TrailerCard(trailer, LandscapeWidth, { context.openTrailer(trailer.url) }, Modifier.focusKey(memory, "row/${trailer.id}"))
            }
        }
    }
    val cast = work.credits.filter { it.knownRole == CreditRole.Actor }.take(20)
    if (cast.isNotEmpty()) {
        extraRow(R.string.cast, nextTop()) {
            items(cast, key = { it.personId + (it.character ?: "") }) { credit ->
                PersonCard(credit.name, credit.photo, credit.character, { navigator.push(Screen.Person(credit.personId)) }, Modifier.focusKey(memory, "cast/${credit.personId}"))
            }
        }
    }
    val extras = work.otherExtras()
    if (extras.isNotEmpty()) {
        extraRow(R.string.extras, nextTop()) {
            items(extras, key = { it.id }) { w ->
                WorkCard(w, CardShape.Landscape, LandscapeWidth, { navigator.push(Screen.Work(w.id)) }, Modifier.focusKey(memory, "row/${w.id}"))
            }
        }
    }
    if (work.collections.isNotEmpty()) {
        extraRow(R.string.collections, nextTop()) {
            items(work.collections, key = { it.id }) { c ->
                CollectionCard(c, PosterWidth, { navigator.push(Screen.Collection(c.id)) }, Modifier.focusKey(memory, "row/${c.id}"))
            }
        }
    }
}

/** Opens a trailer in the YouTube app. On a TV with nothing to open it, it does nothing. */
private fun Context.openTrailer(url: String) {
    try {
        startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
    } catch (_: ActivityNotFoundException) {
        // No app here plays it.
    }
}

/** A titled row under a hero, raised with its focused card, so the card's glow lies over the row below. */
private fun LazyListScope.extraRow(title: Int, top: Dp, cards: LazyListScope.() -> Unit) {
    item(key = title) {
        Column(Modifier.raisedOnFocus().padding(top = top)) {
            RowTitle(stringResource(title), Modifier.padding(bottom = RowTitleGap))
            CardRow(Modifier.rowPlace(), content = cards)
        }
    }
}

/** "Season 3", or "Specials" for season 0. */
@Composable
private fun seasonName(number: Int?) = if (number == 0) stringResource(R.string.specials) else stringResource(R.string.season_n, number ?: 1)

/** A season's pill: "S3", or "Specials" for season 0. */
@Composable
private fun seasonLabel(number: Int?) = if (number == 0) stringResource(R.string.specials) else stringResource(R.string.season_short, number ?: 1)

/**
 * A series: Next Up as the primary action, the season strip
 * (none for a single season), and the open season's episodes as a row of
 * cards that opens on Next Up, opening on
 * the season whose page this is, or Go to series' [episode]'s, else the
 * season holding Next Up, as [Seasons.openSeason] picks. Its seasons
 * are read with the series; until they're in, placeholders hold their place.
 */
@Composable
private fun SeriesDetail(model: DetailViewModel, page: Page, seasonId: String?, episode: EpisodeInfo?, navigator: Navigator) {
    val memory = rememberFocusMemory()
    val hero = rememberHeroLevels(memory)
    val head = page.head
    val work = page.work
    val seasonsLoader = model.children(head.id)
    val seasonsState by seasonsLoader.state.collectAsStateWithLifecycle()
    val seasons = seasonsState.getOrNull()?.sortedBy { if (it.season?.number == 0) Int.MAX_VALUE else it.season?.number ?: 0 }
    val nextUp = work?.series?.nextUp
    var chosen by rememberSaveable { mutableStateOf<String?>(null) }
    // Which season holds Next Up is known once the series is read.
    val open = seasons?.let { listed ->
        listed.firstOrNull { it.id == chosen } ?: Seasons.openSeason(listed, seasonId, episode, nextUp?.episode, seriesRead = work != null || page.problem != null)
    }
    // The season on screen keeps its row until the chosen one's episodes are ready.
    var shown by remember { mutableStateOf<Pair<WorkSummary, List<WorkSummary>>?>(null) }
    val episodesLoader = open?.let { model.children(it.id) }
    val episodesState = episodesLoader?.state?.collectAsStateWithLifecycle()?.value
    val episodes = episodesState?.getOrNull()
    if (open != null && episodes != null && shown != (open to episodes)) shown = open to episodes
    var confirming by remember { mutableStateOf(false) }
    var confirmingSeason by remember { mutableStateOf<WorkSummary?>(null) }

    val series = work?.series ?: head.series
    val years = series?.lastAirYear?.takeIf { it != head.year }
    val seasonCount = series?.seasonCount ?: seasons?.size
    val meta = listOfNotNull(
        head.year?.let { if (years != null) stringResource(R.string.years_range, it, years) else it.toString() },
        seasonCount?.let { pluralStringResource(R.plurals.seasons, it, it) },
        head.communityRating?.let { "★ ${formatRating(it)}" },
        head.contentRating,
    ).joinToString(" · ")
    val played = head.userState?.played == true
    val myListId by model.myListId.collectAsStateWithLifecycle()
    val onMyList = work.onList(myListId)
    val server = LocalSession.current.session.baseUrl

    val pills = remember { FocusRequester() }
    val cards = remember { FocusRequester() }
    val strip = seasons != null && seasons.size > 1
    val current = shown
    // Focus can be sent to the cards only while they're there, as a failure may stand in their place.
    val cardsShown = current != null && episodesState?.problemOrNull() == null
    val toCards = { if (cardsShown) cards else FocusRequester.Default }
    hero.below = if (strip) pills else cards.takeIf { cardsShown }

    DetailFrame(head.images.backdrop) {
        DetailList(top = HeroTop, bottom = SeriesRoomBelow) {
            item {
                Column(Modifier.bringsTopIntoView()) {
                    HeroTitle(head.title)
                    Spacer(Modifier.height(HeroGap))
                    HeroLine(meta)
                    Spacer(Modifier.height(HeroGap))
                    Row(Modifier.heroButtons(hero), horizontalArrangement = Arrangement.spacedBy(HeroGapTight), verticalAlignment = Alignment.CenterVertically) {
                        // Until the series is read, the button holds Next Up's place, and focus, as Play.
                        if (nextUp != null || work == null) {
                            val e = nextUp?.episode
                            val position = nextUp?.userState?.positionMs
                            PlayButton(
                                when {
                                    nextUp == null -> stringResource(R.string.play)
                                    position != null -> stringResource(R.string.resume_at, display.clock(position.toLong()))
                                    else -> stringResource(R.string.play_episode, e?.seasonNumber ?: 0, e?.number ?: 0)
                                },
                                hero,
                            ) { nextUp?.let { navigator.push(Screen.Player(it.id)) } }
                            nextUp?.takeIf { it.userState?.positionMs != null }?.let { RestartButton(hero) { navigator.push(Screen.Player(it.id, fromStart = true)) } }
                        }
                        HeroIconButton(if (onMyList) KivoIcon.ListCheck else KivoIcon.ListPlus, stringResource(if (onMyList) R.string.remove_from_my_list else R.string.add_to_my_list), hero, "myList", onMyList) { model.setOnMyList(head.id, !onMyList) }
                        HeroIconButton(KivoIcon.Check, stringResource(if (played) R.string.mark_unplayed else R.string.mark_played), hero, "played", played) {
                            if (played) model.setPlayed(head.id, false) else confirming = true
                        }
                        MoreButton(head, hero, navigator)
                    }
                    Spacer(Modifier.height(HeroGapTight))
                    // What the button plays: "S3 · E4 · Reload · 30 min left".
                    val left = nextUp?.userState?.positionMs?.let { p -> nextUp.durationMs?.let { d -> display.timeLeft((d - p).toLong()) } }
                    HeroLine(if (nextUp == null) "" else listOfNotNull(nextUp.episode?.let(display::episodeCode), nextUp.title, left).joinToString(" · "))
                    Spacer(Modifier.height(HeroGap))
                    OverviewSlot(hero, page, head.title, work?.overview)
                    // Room for its focus outline, and enough that it's wholly off the screen with the episodes at the top.
                    Spacer(Modifier.height(24.dp))
                }
            }
            val seasonsProblem = seasonsState.problemOrNull()
            val episodesProblem = episodesState?.problemOrNull()
            if (seasonsProblem != null) {
                item { InlineFailed(seasonsProblem, server, seasonsLoader::retry) }
            } else if (seasons?.isEmpty() != true) {
                item(key = "episodes") {
                    // Its strip, heading and cards come to the screen's top together, the hero wholly away rather than half.
                    val above = with(LocalDensity.current) { (SafeTop - HeroTop).toPx() }
                    Column(Modifier.bringsTopIntoView(above).raisedOnFocus()) {
                        when {
                            strip -> {
                                SeasonStrip(seasons.orEmpty(), open, memory, pills, toCards) { chosen = it }
                                Spacer(Modifier.height(CardsUnderHeading))
                            }
                            // As many seasons as the series is known to have; a single season has no strip.
                            seasons == null && (series?.seasonCount ?: 0) > 1 -> {
                                Row(horizontalArrangement = Arrangement.spacedBy(HeroGapTight)) {
                                    repeat(minOf(series?.seasonCount ?: 0, PLACEHOLDERS)) { SkeletonBlock(Modifier.width(60.dp).height(PillHeight), radius = KivoTokens.Radius.full) }
                                }
                                Spacer(Modifier.height(CardsUnderHeading))
                            }
                        }
                        val heading = current?.first ?: open
                        SeasonHeading(heading, strip, memory, pills, toCards) { season ->
                            when {
                                season.userState?.played == true -> model.setPlayed(season.id, false)
                                (season.season?.unplayedCount ?: 0) > 1 -> confirmingSeason = season
                                else -> model.setPlayed(season.id, true)
                            }
                        }
                        when {
                            episodesProblem != null -> InlineFailed(episodesProblem, server, episodesLoader::retry, Modifier.padding(top = CardsUnderHeading))
                            current != null -> key(current.first.id) {
                                EpisodeCardRow(
                                    current.second,
                                    entry = nextUp?.id,
                                    currentId = null,
                                    memory = memory,
                                    group = cards,
                                    navigator = navigator,
                                    modifier = Modifier.rowPlace(top = FocusBleed - CardsUnderHeading),
                                    up = { if (strip) memory.requester(SeasonPlayedKey) else FocusRequester.Default },
                                )
                            }
                            else -> EpisodeCardRowSkeleton(Modifier.padding(top = CardsUnderHeading))
                        }
                    }
                }
            }
            if (work != null) extraRows(work, memory, navigator)
        }
    }
    if (confirming) {
        val count = series?.unplayedCount ?: 0
        ChoicePanel(
            title = pluralStringResource(R.plurals.mark_episodes_played, count, count),
            choices = listOf(Choice("no", stringResource(R.string.cancel)), Choice("yes", stringResource(R.string.mark_played))),
            selected = "no",
            onSelect = {
                confirming = false
                if (it == "yes") model.setPlayed(head.id, true)
            },
            onDismiss = { confirming = false },
        )
    }
    confirmingSeason?.let { season ->
        val count = season.season?.unplayedCount ?: 0
        ChoicePanel(
            title = pluralStringResource(R.plurals.mark_episodes_played, count, count),
            choices = listOf(Choice("no", stringResource(R.string.cancel)), Choice("yes", stringResource(R.string.mark_played))),
            selected = "no",
            onSelect = {
                confirmingSeason = null
                if (it == "yes") model.setPlayed(season.id, true)
            },
            onDismiss = { confirmingSeason = null },
        )
    }
    LaunchedEffect(Unit) { memory.restore(if (nextUp != null || work == null) "play" else "myList") }
    if (work != null && nextUp == null) LaunchedEffect(Unit) { memory.replaced("play", "myList") }
}

/** Placeholders shown at most for a list being read: about as many as fit on the screen. */
private const val PLACEHOLDERS = 6

/** The focus key of the open season's watched check. */
private const val SeasonPlayedKey = "season-played"

/**
 * The seasons as pills, the open one outlined in the accent. Focus comes in
 * on the open one, and Down from any goes to [below], past the watched check.
 */
@Composable
private fun SeasonStrip(seasons: List<WorkSummary>, open: WorkSummary?, memory: FocusMemory, group: FocusRequester, below: () -> FocusRequester, onOpen: (String) -> Unit) {
    val state = rememberLazyListState()
    Box(
        Modifier
            .rowPlace(top = FocusBleed, bottom = FocusBleed)
            .focusRequester(group)
            .focusProperties {
                onEnter = {
                    val shown = state.layoutInfo.visibleItemsInfo.mapNotNull { it.key as? String }
                    rowEntry(shown, null, open?.id)?.let { memory.requester("season/$it").requestFocus() }
                }
            }
            .focusGroup(),
    ) {
        CardRow(spacing = HeroGapTight, state = state) {
            items(seasons, key = { it.id }) { s ->
                SeasonChip(s, s.id == open?.id, Modifier.focusProperties { down = below() }.focusKey(memory, "season/${s.id}")) { onOpen(s.id) }
            }
        }
    }
}

/**
 * The open season's name, with its watched check when there's a strip above
 * it, which Up from the cards lands on, and how many episodes it has at the
 * right.
 */
@Composable
private fun SeasonHeading(season: WorkSummary?, strip: Boolean, memory: FocusMemory, pills: FocusRequester, below: () -> FocusRequester, onPlayed: (WorkSummary) -> Unit) {
    val colors = Kivo.colors
    Row(Modifier.width(WideWidth).height(ButtonHeight), verticalAlignment = Alignment.CenterVertically) {
        when {
            !strip && season != null -> Text(stringResource(R.string.episodes), style = TvType.large, color = colors.text)
            season != null -> Text(seasonName(season.season?.number), style = TvType.large, color = colors.text)
            else -> SkeletonLine(160.dp, TvType.large)
        }
        if (strip && season != null) {
            val played = season.userState?.played == true
            Spacer(Modifier.width(HeroGapTight))
            IconButton(
                KivoIcon.Check,
                stringResource(if (played) R.string.mark_unplayed else R.string.mark_played),
                { onPlayed(season) },
                Modifier
                    .focusProperties {
                        up = pills
                        down = below()
                    }
                    .focusKey(memory, SeasonPlayedKey),
                on = played,
                size = ButtonHeight,
                corners = KivoTokens.Radius.md,
                container = Color.Transparent,
            )
        }
        Spacer(Modifier.weight(1f))
        season?.season?.episodeCount?.let {
            Text(pluralStringResource(R.plurals.episodes, it, it), style = TvType.small, color = colors.textMuted)
        }
    }
}

@Composable
private fun SeasonChip(season: WorkSummary, current: Boolean, modifier: Modifier, onClick: () -> Unit) {
    Chip(seasonLabel(season.season?.number), current, onClick, modifier, height = PillHeight) {
        val colors = Kivo.colors
        val unplayed = season.season?.unplayedCount ?: 0
        if (season.userState?.played == true) {
            KivoIconView(KivoIcon.Check, Modifier.padding(start = Kivo.space(2)), size = 16.dp, tint = colors.accent)
        } else if (unplayed > 0) {
            Text("$unplayed", style = TvType.small, color = colors.textMuted, modifier = Modifier.padding(start = Kivo.space(2)))
        }
    }
}

/**
 * An episode: its still behind it, one screen that doesn't scroll to reach
 * its season. The series' name, leading back to the series; its title on one
 * line and "S3 · E4 · date · runtime · Ends at"; Play or Resume with the track
 * pickers along its row; up to six lines of description; and its season's
 * row of cards, opening on this episode, whose neighbours are a move away.
 */
@Composable
private fun EpisodeDetail(model: DetailViewModel, page: Page, navigator: Navigator) {
    val memory = rememberFocusMemory()
    val hero = rememberHeroLevels(memory)
    var choices by rememberChoices()
    val head = page.head
    val work = page.work
    val e = head.episode
    val cards = remember { FocusRequester() }
    // All of it stays where it opened, wherever focus moves in it, its season's row too.
    val atRest = with(LocalDensity.current) { (SafeTop - HeroTop).toPx() }
    DetailFrame(head.images.still ?: head.images.backdrop) {
        DetailList(top = SafeTop, pivot = HeroTop) {
            item {
                Column(Modifier.bringsTopIntoView(atRest)) {
                    Box(Modifier.height(20.dp)) {
                        e?.let { SeriesLink(it, hero, navigator) }
                    }
                    Spacer(Modifier.height(HeroGapTight))
                    HeroTitle(head.title, maxLines = 1, width = WideWidth)
                    Spacer(Modifier.height(HeroGapTight))
                    EpisodeLine(head)
                    Spacer(Modifier.height(HeroGapTight))
                    ActionRow(model, head, work, choices, hero, navigator, chips = {
                        if (work != null) Pickers(work, choices, hero, chips = true) { choices = it }
                    })
                    Spacer(Modifier.height(HeroGap))
                    OverviewSlot(hero, page, head.title, work?.overview, lines = 5, width = EpisodeBlockWidth, textWidth = EpisodeTextWidth)
                    Spacer(Modifier.height(HeroGap))
                    e?.seasonId?.let { seasonId ->
                        val loader = model.children(seasonId)
                        val state = loader.state.collectAsStateWithLifecycle().value
                        val episodes = state.getOrNull()
                        val problem = state.problemOrNull()
                        hero.below = cards.takeIf { episodes != null }
                        Text(
                            if (e.seasonNumber == 0) stringResource(R.string.specials) else stringResource(R.string.season_n, e.seasonNumber),
                            style = TvType.large,
                            color = Kivo.colors.text,
                        )
                        when {
                            episodes != null -> EpisodeCardRow(
                                episodes,
                                entry = head.id,
                                currentId = head.id,
                                memory = memory,
                                group = cards,
                                navigator = navigator,
                                modifier = Modifier.rowPlace(top = FocusBleed - CardsUnderHeading),
                            )
                            problem != null -> InlineFailed(problem, LocalSession.current.session.baseUrl, loader::retry, Modifier.padding(top = CardsUnderHeading))
                            else -> EpisodeCardRowSkeleton(Modifier.padding(top = CardsUnderHeading))
                        }
                    }
                }
            }
            if (work != null) extraRows(work, memory, navigator)
        }
    }
    LaunchedEffect(Unit) { memory.restore(if (playable(head, work, choices)) "play" else "myList") }
}

/** The series' name over an episode's title: its own stop, leading back to the series, lit with focus. */
@Composable
private fun SeriesLink(episode: EpisodeInfo, hero: HeroLevels, navigator: Navigator) {
    val colors = Kivo.colors
    Surface(
        onClick = { navigator.push(Screen.Work(episode.seriesId, episode)) },
        modifier = Modifier
            .offset(x = -SeriesLinkPadding)
            .focusProperties { down = hero.buttons }
            .focusKey(hero.memory, "series"),
        shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(KivoTokens.Radius.sm)),
        colors = ClickableSurfaceDefaults.colors(
            containerColor = Color.Transparent,
            contentColor = colors.text,
            focusedContainerColor = Lit.container,
            focusedContentColor = Lit.content,
        ),
        scale = ClickableSurfaceDefaults.scale(focusedScale = 1f),
    ) {
        Text(episode.seriesTitle, style = ControlLabel, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(horizontal = SeriesLinkPadding))
    }
}

/** How far the series' name's focus reaches past its text, which lines up with the title. */
private val SeriesLinkPadding = 6.dp

/** An episode's details, "S3 · E4 · Feb 4, 2017 · 44m · ★ 8.5 · TV-14 · Ends at 4:57 PM"; only this follows the clock. */
@Composable
private fun EpisodeLine(head: WorkSummary) {
    val e = head.episode
    HeroLine(
        listOfNotNull(
            e?.let(display::episodeCode),
            e?.airDate?.let { java.time.LocalDate.parse(it.toString()).format(java.time.format.DateTimeFormatter.ofLocalizedDate(java.time.format.FormatStyle.MEDIUM)) },
            head.durationMs?.let { display.runtime(it.toLong()) },
            head.communityRating?.let { "★ ${formatRating(it)}" },
            head.contentRating,
            head.durationMs?.let { timeLeftLine(head.userState?.positionMs, it) },
        ).joinToString(" · "),
    )
}

/** A collection's page's read, refetched as the libraries or played marks change. */
class CollectionViewModel(session: ServerSession, id: String) : ViewModel() {
    /** Its card as the grids and pages read lately showed it, which shows at once; null when none did. */
    val seed: CollectionSummary? = session.session.library.knownCollection(id)

    val collection = Loader(viewModelScope) { session.session.library.collection(id) }
}

/**
 * A collection, a movie collection or a folder of an other-videos library:
 * its name and count, at once when a card showed them, over its backdrop
 * once read, then its root works as posters, as many placeholders as it's known
 * to hold until they're in; with none, focus goes to the rail.
 */
@Composable
fun CollectionScreen(session: ServerSession, id: String, navigator: Navigator) {
    val model = viewModel(key = id) { CollectionViewModel(session, id) }
    val state by model.collection.state.collectAsStateWithLifecycle()
    val collection = state.getOrNull()
    val problem = state.problemOrNull()
    val seed = model.seed
    val name = collection?.name ?: seed?.name
    if (name == null && problem != null) {
        Failed(problem, session.session.baseUrl, model.collection::retry, session.graph::changeServer)
        return
    }
    val memory = rememberFocusMemory()
    val members = collection?.items
    SkeletonGate(name, { SkeletonHero(Modifier.padding(start = 24.dp, top = 120.dp)) }) { title ->
        if (members != null && members.isEmpty()) FocusRail()
        DetailFrame(collection?.images?.backdrop) {
            DetailList(top = 120.dp) {
                item {
                    val count = members?.size ?: seed?.workCount ?: 0
                    HeroText(title, collectionCount(count, collection?.source ?: seed?.source))
                }
                item {
                    when {
                        members != null -> CardRow(Modifier.padding(top = Kivo.space(8))) {
                            items(members, key = { it.id }) { w ->
                                WorkCard(w, CardShape.Poster, PosterWidth, { navigator.push(Screen.Work(w.id)) }, Modifier.focusKey(memory, w.id))
                            }
                        }
                        problem != null -> InlineFailed(problem, session.session.baseUrl, model.collection::retry, Modifier.padding(top = Kivo.space(8)))
                        else -> Row(Modifier.padding(top = Kivo.space(8)), horizontalArrangement = Arrangement.spacedBy(CardGap)) {
                            repeat(minOf(seed?.workCount ?: PLACEHOLDERS, PLACEHOLDERS)) { SkeletonCard(CardShape.Poster, PosterWidth) }
                        }
                    }
                }
            }
        }
    }
    LaunchedEffect(members?.isNotEmpty()) { members?.firstOrNull()?.let { memory.restore(it.id) } }
}
