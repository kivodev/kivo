package com.mmcdole.kivo.android.detail

import androidx.compose.animation.Crossfade
import androidx.compose.animation.core.updateTransition
import androidx.compose.animation.ExperimentalAnimationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
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
import androidx.tv.material3.Card
import androidx.tv.material3.CardDefaults
import androidx.tv.material3.ClickableSurfaceDefaults
import androidx.tv.material3.Surface
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.Artwork
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
import com.mmcdole.kivo.android.ui.ProgressBar
import com.mmcdole.kivo.android.ui.RowTitle
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.ValueRow
import com.mmcdole.kivo.android.ui.WorkCard
import com.mmcdole.kivo.android.ui.LocalSession
import com.mmcdole.kivo.android.ui.Overview
import com.mmcdole.kivo.android.ui.LocalWorkMenu
import com.mmcdole.kivo.android.ui.menuKey
import com.mmcdole.kivo.android.ui.fadeSpec
import com.mmcdole.kivo.android.ui.display
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.focusRing
import com.mmcdole.kivo.android.ui.formatRating
import com.mmcdole.kivo.android.ui.overlay
import com.mmcdole.kivo.android.ui.progressOf
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import com.mmcdole.kivo.api.models.EpisodeInfo
import com.mmcdole.kivo.api.models.CollectionSummary
import com.mmcdole.kivo.api.models.ImageRef
import com.mmcdole.kivo.api.models.Track
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.data.Seasons
import com.mmcdole.kivo.core.data.SubtitleSearch
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
            page.head.kind == "series" -> SeriesDetail(model, page, null, episode, navigator)
            page.head.kind == "season" && season != null ->
                Shown(model.other(season.seriesId), model.seriesSeed(page.head), session) { SeriesDetail(model, it, page.head.id, null, navigator) }
            page.head.kind == "episode" -> EpisodeDetail(model, page, navigator)
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

/** The page's frame: the backdrop at the top, darkened at the left and fading into the background. */
@Composable
private fun DetailFrame(backdrop: ImageRef?, content: @Composable () -> Unit) {
    val colors = Kivo.colors
    Box(Modifier.fillMaxSize()) {
        if (backdrop != null) {
            Box(
                Modifier
                    .fillMaxWidth()
                    .overlay(
                        Brush.horizontalGradient(listOf(colors.scrim, colors.scrim, colors.scrimSoft)),
                        Brush.verticalGradient(listOf(Color.Transparent, colors.bg)),
                    ),
            ) {
                Artwork(backdrop, "", KivoTokens.Aspect.landscape, 880.dp, Modifier.fillMaxWidth(), corners = 0.dp)
            }
        }
        content()
    }
}

/**
 * A detail page's list: what takes focus sits [top] from the top of the
 * screen, where the page's title starts, as the rows move under it; focus
 * in the hero keeps the whole hero in view.
 */
@Composable
private fun DetailList(top: Dp, content: LazyListScope.() -> Unit) {
    RowsHoldPlace(top) {
        LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(start = 24.dp, top = top, end = SafeEdge, bottom = SafeEdge), content = content)
    }
}

/** The hero's text: the title, its lines and the overview, three lines at most, which OK opens in full. */
@Composable
private fun HeroText(eyebrow: (@Composable () -> Unit)?, heading: String, meta: String, overview: String?) {
    val colors = Kivo.colors
    Column(Modifier.widthIn(max = 600.dp), verticalArrangement = Arrangement.spacedBy(Kivo.space(2))) {
        eyebrow?.invoke()
        Text(heading, style = TvType.title, color = colors.text, maxLines = 2, overflow = TextOverflow.Ellipsis)
        if (meta.isNotEmpty()) Text(meta, style = TvType.body, color = colors.textMuted)
        if (!overview.isNullOrBlank()) OverviewText(heading, overview)
    }
}

/** An overview whose text lines up with the title above; its focus background reaches past it. */
@Composable
private fun OverviewText(title: String, text: String) {
    Overview(title, text, Modifier.offset(x = -Kivo.space(3)).widthIn(max = 640.dp))
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
    var choices by rememberChoices()
    val head = page.head
    val work = page.work
    DetailFrame(head.images.backdrop) {
        DetailList(top = 120.dp) {
            item {
                Column(Modifier.bringsTopIntoView(), verticalArrangement = Arrangement.spacedBy(Kivo.space(5))) {
                    HeroText(null, labelledTitle(head.title, head.movie), metaLine(head, work, display), null)
                    Column {
                        ActionRow(model, head, work, choices, memory, navigator)
                        TimeLeft(head.userState?.positionMs, head.durationMs)
                    }
                    if (work != null) {
                        Pickers(work, choices) { choices = it }
                        work.overview?.takeIf { it.isNotBlank() }?.let { OverviewText(work.title, it) }
                    }
                    Pending(page)
                }
            }
            if (work != null) extraRows(work, memory, navigator)
        }
    }
    LaunchedEffect(Unit) { memory.restore(if (playable(head, work, choices)) "play" else "favorite") }
}

/** Whether the work, in the version selected, can play; before its versions are read, whether its card says it can. */
private fun playable(head: WorkSummary, work: WorkDetail?, choices: Choices) = if (work == null) {
    head.available
} else {
    work.available && work.selectedVersion(choices.versionId)?.available == true
}

/**
 * Play or Resume with the time, and beside Resume a round Play from
 * beginning; then Favorite, Played and the ⋯ menu; "This file is
 * missing" in Play's place when no version can play. They work from the
 * card before the page is read.
 * [before] and [after] are the episode page's Previous and Next.
 */
@Composable
private fun ActionRow(
    model: DetailViewModel,
    work: WorkSummary,
    read: WorkDetail?,
    choices: Choices,
    memory: FocusMemory,
    navigator: Navigator,
    before: (@Composable () -> Unit)? = null,
    after: (@Composable () -> Unit)? = null,
) {
    val played = work.userState?.played == true
    val favorite = work.userState?.favorite == true
    val playable = playable(work, read, choices)
    val position = if (read != null) read.resumeAt(choices.versionId) else work.userState?.positionMs
    fun play(fromStart: Boolean) = navigator.push(
        Screen.Player(work.id, read?.selectedVersion(choices.versionId)?.id ?: choices.versionId, choices.audioTrackId, choices.subtitleTrackId, fromStart),
    )
    Row(horizontalArrangement = Arrangement.spacedBy(Kivo.space(3)), verticalAlignment = Alignment.CenterVertically) {
        before?.invoke()
        if (!playable) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                KivoIconView(KivoIcon.Alert, tint = Kivo.colors.warning)
                Text(stringResource(R.string.file_missing), style = TvType.bodyStrong, color = Kivo.colors.warning, modifier = Modifier.padding(start = Kivo.space(2)))
            }
        } else {
            KivoButton(
                text = if (position != null) stringResource(R.string.resume_at, display.clock(position.toLong())) else stringResource(R.string.play),
                onClick = { play(fromStart = false) },
                icon = KivoIcon.Play,
                primary = true,
                modifier = Modifier.focusKey(memory, "play").testTag("play"),
            )
            if (position != null) RestartButton(memory) { play(fromStart = true) }
        }
        IconButton(
            KivoIcon.Heart,
            stringResource(if (favorite) R.string.remove_favorite else R.string.add_favorite),
            { model.setFavorite(work.id, !favorite) },
            Modifier.focusKey(memory, "favorite"),
            on = favorite,
        )
        IconButton(
            KivoIcon.Check,
            stringResource(if (played) R.string.mark_unplayed else R.string.mark_played),
            { model.setPlayed(work.id, !played) },
            Modifier.focusKey(memory, "played"),
            on = played,
        )
        MoreButton(work, memory, navigator)
        after?.invoke()
    }
}

/** Play from beginning, a round button beside Resume. */
@Composable
private fun RestartButton(memory: FocusMemory, onClick: () -> Unit) {
    IconButton(KivoIcon.Replay, stringResource(R.string.play_from_beginning), onClick, Modifier.focusKey(memory, "restart").testTag("restart"))
}

/** The hero's ⋯: Go to series for an episode, and Go to library. */
@Composable
private fun MoreButton(work: WorkSummary, memory: FocusMemory, navigator: Navigator) {
    var open by remember { mutableStateOf(false) }
    val seriesId = work.episode?.seriesId
    val libraryId = work.libraryId
    val choices = buildList {
        if (seriesId != null) add(Choice("series", stringResource(R.string.go_to_series)))
        if (libraryId != null) add(Choice("library", stringResource(R.string.go_to_library)))
    }
    if (choices.isEmpty()) return
    IconButton(KivoIcon.More, stringResource(R.string.more_actions), { open = true }, Modifier.focusKey(memory, "more"))
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
    if (durationMs == null) return
    val leftMs = (durationMs - (positionMs ?: 0)).toLong()
    Text(
        listOfNotNull(display.timeLeft(leftMs).takeIf { positionMs != null }, endsAt(leftMs)).joinToString(" · "),
        style = TvType.body,
        color = Kivo.colors.textMuted,
        modifier = Modifier.padding(top = Kivo.space(2)),
    )
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
 * which shows among the tracks until the work is read again with it.
 */
@Composable
private fun Pickers(read: WorkDetail, choices: Choices, onChange: (Choices) -> Unit) {
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

    // Each picker's text lines up with the title above; its focus background reaches past it.
    Row(Modifier.offset(x = -Kivo.space(3)), horizontalArrangement = Arrangement.spacedBy(Kivo.space(2))) {
        if (work.versions.size > 1) {
            ValueRow(stringResource(R.string.version), version.name, { open = Picker.Version }, Modifier.widthIn(max = 220.dp))
        }
        if (tracks.choosable) {
            // What plays: the track chosen, else the one Automatic plays.
            val audio = choices.audioTrackId ?: automatic.audio
            val subtitle = choices.subtitleTrackId ?: automatic.subtitle
            val value = listOfNotNull(
                tracks.audio.firstOrNull { it.id == audio }?.name.takeIf { tracks.audio.size > 1 },
                (if (subtitle == Subtitles.OFF) offLabel else tracks.subtitles.firstOrNull { it.id == subtitle }?.name)
                    .takeIf { tracks.subtitles.isNotEmpty() || tracks.searchable },
            ).joinToString(" / ")
            ValueRow(stringResource(R.string.audio_and_subtitles), value, { open = Picker.Tracks }, Modifier.widthIn(max = 420.dp).testTag("tracks"))
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

/** Cast, extras and collections under the hero; rows with nothing in them are left out. */
private fun androidx.compose.foundation.lazy.LazyListScope.extraRows(work: WorkDetail, memory: FocusMemory, navigator: Navigator) {
    val cast = work.credits.filter { it.role == "actor" }.take(20)
    if (cast.isNotEmpty()) {
        item {
            Column(Modifier.padding(top = Kivo.space(8))) {
                RowTitle(stringResource(R.string.cast), Modifier.padding(bottom = Kivo.space(3)))
                CardRow {
                    items(cast, key = { it.personId + (it.character ?: "") }) { credit ->
                        PersonCard(credit.name, credit.photo, credit.character, { navigator.push(Screen.Person(credit.personId)) }, Modifier.focusKey(memory, "cast/${credit.personId}"))
                    }
                }
            }
        }
    }
    cardRow(R.string.extras, work.extras, CardShape.Landscape, memory, navigator)
    if (work.collections.isNotEmpty()) {
        item {
            Column(Modifier.padding(top = Kivo.space(8))) {
                RowTitle(stringResource(R.string.collections), Modifier.padding(bottom = Kivo.space(3)))
                CardRow {
                    items(work.collections, key = { it.id }) { c ->
                        CollectionCard(c, PosterWidth, { navigator.push(Screen.Collection(c.id)) }, Modifier.focusKey(memory, "row/${c.id}"))
                    }
                }
            }
        }
    }
}

private fun androidx.compose.foundation.lazy.LazyListScope.cardRow(
    title: Int,
    works: List<WorkSummary>,
    shape: CardShape,
    memory: FocusMemory,
    navigator: Navigator,
) {
    if (works.isEmpty()) return
    item {
        Column(Modifier.padding(top = Kivo.space(8))) {
            RowTitle(stringResource(title), Modifier.padding(bottom = Kivo.space(3)))
            CardRow {
                items(works, key = { it.id }) { w ->
                    WorkCard(w, shape, if (shape == CardShape.Landscape) LandscapeWidth else PosterWidth, { navigator.push(Screen.Work(w.id)) }, Modifier.focusKey(memory, "row/${w.id}"))
                }
            }
        }
    }
}

/** "Season 3", or "Specials" for season 0. */
@Composable
private fun seasonName(number: Int?) = if (number == 0) stringResource(R.string.specials) else stringResource(R.string.season_n, number ?: 1)

/**
 * A series: Next Up as the primary action, the season strip
 * (none for a single season), and the open season's episodes, opening on
 * the season whose page this is, or Go to series' [episode]'s, else the
 * season holding Next Up, as [Seasons.openSeason] picks. Its seasons
 * are read with the series; until they're in, placeholders hold their place.
 */
@OptIn(ExperimentalAnimationApi::class)
@Composable
private fun SeriesDetail(model: DetailViewModel, page: Page, seasonId: String?, episode: EpisodeInfo?, navigator: Navigator) {
    val memory = rememberFocusMemory()
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
    // The season on screen keeps its rows until the chosen one's episodes are ready.
    var shown by remember { mutableStateOf<Pair<WorkSummary, List<WorkSummary>>?>(null) }
    val episodesLoader = open?.let { model.children(it.id) }
    val episodesState = episodesLoader?.state?.collectAsStateWithLifecycle()?.value
    val episodes = episodesState?.getOrNull()
    if (open != null && episodes != null && shown != (open to episodes)) shown = open to episodes
    var confirming by remember { mutableStateOf(false) }

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
    val favorite = head.userState?.favorite == true
    val server = LocalSession.current.session.baseUrl

    DetailFrame(head.images.backdrop) {
        DetailList(top = 120.dp) {
            item {
                Column(Modifier.bringsTopIntoView(), verticalArrangement = Arrangement.spacedBy(Kivo.space(5))) {
                    HeroText(null, head.title, meta, work?.overview)
                    if (work == null && page.problem == null) Pending(page)
                    Column {
                        Row(horizontalArrangement = Arrangement.spacedBy(Kivo.space(3)), verticalAlignment = Alignment.CenterVertically) {
                            // Until the series is read, the button holds Next Up's place, and focus, as Play.
                            if (nextUp != null || work == null) {
                                val e = nextUp?.episode
                                val position = nextUp?.userState?.positionMs
                                KivoButton(
                                    text = when {
                                        nextUp == null -> stringResource(R.string.play)
                                        position != null -> stringResource(R.string.resume_at, display.clock(position.toLong()))
                                        else -> stringResource(R.string.play_episode, e?.seasonNumber ?: 0, e?.number ?: 0)
                                    },
                                    onClick = { nextUp?.let { navigator.push(Screen.Player(it.id)) } },
                                    icon = KivoIcon.Play,
                                    primary = true,
                                    modifier = Modifier.focusKey(memory, "play").testTag("play"),
                                )
                                nextUp?.takeIf { it.userState?.positionMs != null }?.let { RestartButton(memory) { navigator.push(Screen.Player(it.id, fromStart = true)) } }
                            }
                            IconButton(KivoIcon.Heart, stringResource(if (favorite) R.string.remove_favorite else R.string.add_favorite), { model.setFavorite(head.id, !favorite) }, Modifier.focusKey(memory, "favorite"), on = favorite)
                            IconButton(
                                KivoIcon.Check,
                                stringResource(if (played) R.string.mark_unplayed else R.string.mark_played),
                                { if (played) model.setPlayed(head.id, false) else confirming = true },
                                Modifier.focusKey(memory, "played"),
                                on = played,
                            )
                            MoreButton(head, memory, navigator)
                        }
                        // What the button plays: "S3 · E4 · Reload · 30 min left".
                        if (nextUp != null) {
                            val left = nextUp.userState?.positionMs?.let { p -> nextUp.durationMs?.let { d -> display.timeLeft((d - p).toLong()) } }
                            Text(
                                listOfNotNull(nextUp.episode?.let(display::episodeCode), nextUp.title, left).joinToString(" · "),
                                style = TvType.body,
                                color = Kivo.colors.textMuted,
                                modifier = Modifier.padding(top = Kivo.space(2)),
                            )
                        }
                    }
                    if (page.problem != null) Pending(page)
                }
            }
            val seasonsProblem = seasonsState.problemOrNull()
            when {
                seasonsProblem != null -> item {
                    InlineFailed(seasonsProblem, server, seasonsLoader::retry, Modifier.padding(top = Kivo.space(8)))
                }
                // As many seasons as the series is known to have; a single season has no strip.
                seasons == null && (series?.seasonCount ?: 0) > 1 -> item {
                    Row(Modifier.padding(top = Kivo.space(8)), horizontalArrangement = Arrangement.spacedBy(Kivo.space(2))) {
                        repeat(minOf(series?.seasonCount ?: 0, PLACEHOLDERS)) { SkeletonBlock(Modifier.width(120.dp).height(40.dp), radius = KivoTokens.Radius.full) }
                    }
                }
                seasons != null && seasons.size > 1 -> item {
                    CardRow(Modifier.padding(top = Kivo.space(8)), spacing = Kivo.space(2)) {
                        items(seasons, key = { it.id }) { s ->
                            SeasonChip(s, s.id == open?.id, Modifier.focusKey(memory, "season/${s.id}")) { chosen = s.id }
                        }
                    }
                }
            }
            val episodesProblem = episodesState?.problemOrNull()
            val current = shown
            when {
                episodesProblem != null -> item(key = "episodes") {
                    InlineFailed(episodesProblem, server, episodesLoader::retry, Modifier.padding(top = Kivo.space(6)))
                }
                current != null -> item(key = "episodes") {
                    updateTransition(current, label = "season").Crossfade(animationSpec = fadeSpec(), contentKey = { it.first.id }) { (season, episodes) ->
                        Column(Modifier.padding(top = Kivo.space(6))) {
                            EpisodesTitle(if (seasons.orEmpty().size > 1) seasonName(season.season?.number) else stringResource(R.string.episodes))
                            EpisodeRows(episodes, null, memory, navigator)
                        }
                    }
                }
                seasonsProblem == null && seasons?.isEmpty() != true -> item(key = "episodes") {
                    Column(Modifier.padding(top = Kivo.space(6))) {
                        when {
                            seasons?.size == 1 -> EpisodesTitle(stringResource(R.string.episodes))
                            open != null -> EpisodesTitle(seasonName(open.season?.number))
                            else -> SkeletonLine(160.dp, TvType.large, Modifier.padding(bottom = Kivo.space(2)))
                        }
                        EpisodeRowsSkeleton(open?.season?.episodeCount)
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
    LaunchedEffect(Unit) { memory.restore(if (nextUp != null || work == null) "play" else "favorite") }
    if (work != null && nextUp == null) LaunchedEffect(Unit) { memory.replaced("play", "favorite") }
}

/** Placeholders shown at most for a list being read: about as many as fit on the screen. */
private const val PLACEHOLDERS = 6

@Composable
private fun EpisodesTitle(text: String) {
    Text(text, style = TvType.large, color = Kivo.colors.text, modifier = Modifier.padding(bottom = Kivo.space(2)))
}

/** A season's episode rows while they're read: as many as it has, of those that fit, else a few. */
@Composable
private fun EpisodeRowsSkeleton(count: Int?) {
    Column(verticalArrangement = Arrangement.spacedBy(Kivo.space(2))) {
        repeat(minOf(count ?: 3, PLACEHOLDERS)) {
            Row(Modifier.padding(Kivo.space(1)), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(Kivo.space(4))) {
                SkeletonBlock(Modifier.width(KivoTokens.Layout.stillWidth).aspectRatio(KivoTokens.Aspect.landscape))
                Column(Modifier.padding(Kivo.space(3))) {
                    SkeletonLine(320.dp, TvType.bodyStrong)
                    SkeletonLine(120.dp, TvType.small)
                }
            }
        }
    }
}

@Composable
private fun SeasonChip(season: WorkSummary, current: Boolean, modifier: Modifier, onClick: () -> Unit) {
    val colors = Kivo.colors
    Chip(seasonName(season.season?.number), current, onClick, modifier) {
        val unplayed = season.season?.unplayedCount ?: 0
        if (season.userState?.played == true) {
            KivoIconView(KivoIcon.Check, Modifier.padding(start = Kivo.space(2)), size = 16.dp, tint = colors.accent)
        } else if (unplayed > 0) {
            Text("$unplayed", style = TvType.small, color = colors.textMuted, modifier = Modifier.padding(start = Kivo.space(2)))
        }
    }
}

/** A season's episodes as rows: the still plays it (resuming one in progress), the rest of the row opens its page. */
@Composable
private fun EpisodeRows(episodes: List<WorkSummary>, currentId: String?, memory: FocusMemory, navigator: Navigator) {
    Column(verticalArrangement = Arrangement.spacedBy(Kivo.space(2))) {
        for (episode in episodes) {
            EpisodeRow(episode, episode.id == currentId, memory, navigator)
        }
    }
}

@Composable
private fun EpisodeRow(episode: WorkSummary, current: Boolean, memory: FocusMemory, navigator: Navigator) {
    val colors = Kivo.colors
    val menu = LocalWorkMenu.current
    val e = episode.episode
    val number = if (e?.endNumber != null && e.endNumber != e.number) "E${e.number}–${e.endNumber}" else "E${e?.number ?: ""}"
    Row(
        Modifier
            .fillMaxWidth()
            .background(if (current) colors.surfaceRaised else Color.Transparent, RoundedCornerShape(KivoTokens.Radius.md))
            .padding(Kivo.space(1)),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(Kivo.space(4)),
    ) {
        Card(
            onClick = { navigator.push(Screen.Player(episode.id)) },
            onLongClick = { menu(episode) },
            modifier = Modifier.width(KivoTokens.Layout.stillWidth).focusKey(memory, "still/${episode.id}").menuKey(episode, menu).testTag("play:${episode.title}"),
            shape = CardDefaults.shape(RoundedCornerShape(KivoTokens.Radius.md)),
            border = CardDefaults.border(focusedBorder = focusRing()),
            scale = CardDefaults.scale(focusedScale = KivoTokens.Tv.cardFocusScale),
        ) {
            Box {
                Artwork(episode.images.still, episode.title, KivoTokens.Aspect.landscape, KivoTokens.Layout.stillWidth)
                Box(Modifier.align(Alignment.Center).size(36.dp).background(colors.playerScrim, RoundedCornerShape(KivoTokens.Radius.full)), contentAlignment = Alignment.Center) {
                    KivoIconView(KivoIcon.Play, size = 18.dp, tint = colors.onPlayer)
                }
                progressOf(episode)?.let { ProgressBar(it, Modifier.align(Alignment.BottomCenter)) }
            }
        }
        Surface(
            onClick = { navigator.push(Screen.Work(episode.id)) },
            onLongClick = { menu(episode) },
            modifier = Modifier.weight(1f).heightIn(min = 64.dp).focusKey(memory, "episode/${episode.id}").menuKey(episode, menu),
            shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(KivoTokens.Radius.md)),
            colors = ClickableSurfaceDefaults.colors(
                containerColor = Color.Transparent,
                contentColor = colors.text,
                focusedContainerColor = colors.surfaceHover,
                focusedContentColor = colors.text,
            ),
            border = ClickableSurfaceDefaults.border(focusedBorder = focusRing()),
            scale = ClickableSurfaceDefaults.scale(focusedScale = 1f),
        ) {
            Row(Modifier.padding(Kivo.space(3)), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("$number  ${episode.title}", style = TvType.bodyStrong, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    val left = episode.userState?.positionMs?.let { p -> episode.durationMs?.let { d -> display.timeLeft((d - p).toLong()) } }
                    val line = listOfNotNull(
                        episode.durationMs?.let { display.runtime(it.toLong()) },
                        left,
                        when (episode.matchState) {
                            "unmatched" -> stringResource(R.string.badge_unmatched)
                            "identifying" -> stringResource(R.string.badge_identifying)
                            else -> null
                        },
                    ).joinToString(" · ")
                    if (line.isNotEmpty()) Text(line, style = TvType.small, color = colors.textMuted)
                }
                if (episode.userState?.played == true) KivoIconView(KivoIcon.Check, size = 20.dp, tint = colors.accent)
            }
        }
    }
}

/**
 * An episode: its still behind it, the series title leading back
 * to the series, "S3 · E4 · Reload", Play or Resume with Previous and Next
 * reached by Left and Right on that row, and its season listed below.
 */
@Composable
private fun EpisodeDetail(model: DetailViewModel, page: Page, navigator: Navigator) {
    val memory = rememberFocusMemory()
    var choices by rememberChoices()
    val head = page.head
    val work = page.work
    val e = head.episode
    val heading = listOfNotNull(
        e?.let(display::episodeCode),
        head.title,
    ).joinToString(" · ")
    val meta = listOfNotNull(
        e?.airDate?.let { java.time.LocalDate.parse(it.toString()).format(java.time.format.DateTimeFormatter.ofLocalizedDate(java.time.format.FormatStyle.MEDIUM)) },
        head.durationMs?.let { display.runtime(it.toLong()) },
        head.communityRating?.let { "★ ${formatRating(it)}" },
        head.contentRating,
    ).joinToString(" · ")
    // Previous and Next are known once the episode is read.
    val links = work?.episode
    DetailFrame(head.images.still ?: head.images.backdrop) {
        DetailList(top = 96.dp) {
            item {
                Column(Modifier.bringsTopIntoView(), verticalArrangement = Arrangement.spacedBy(Kivo.space(5))) {
                    HeroText(
                        eyebrow = e?.let {
                            {
                                KivoButton(it.seriesTitle, { navigator.push(Screen.Work(it.seriesId, it)) }, Modifier.focusKey(memory, "series"))
                            }
                        },
                        heading = heading,
                        meta = meta,
                        overview = null,
                    )
                    Column {
                        ActionRow(
                            model, head, work, choices, memory, navigator,
                            before = links?.previousId?.let { id ->
                                { IconButton(KivoIcon.ChevronLeft, stringResource(R.string.previous_episode), { navigator.replaceWork(id) }, Modifier.focusKey(memory, "previous")) }
                            },
                            after = links?.nextId?.let { id ->
                                { IconButton(KivoIcon.ChevronRight, stringResource(R.string.next_episode), { navigator.replaceWork(id) }, Modifier.focusKey(memory, "next")) }
                            },
                        )
                        TimeLeft(head.userState?.positionMs, head.durationMs)
                    }
                    if (work != null) {
                        Pickers(work, choices) { choices = it }
                        work.overview?.takeIf { it.isNotBlank() }?.let { OverviewText(work.title, it) }
                    }
                    Pending(page)
                }
            }
            val seasonId = e?.seasonId
            if (seasonId != null) {
                item(key = "episodes") {
                    val loader = model.children(seasonId)
                    val state = loader.state.collectAsStateWithLifecycle().value
                    val episodes = state.getOrNull()
                    val problem = state.problemOrNull()
                    Column {
                        Text(
                            if (e.seasonNumber == 0) stringResource(R.string.specials) else stringResource(R.string.season_n, e.seasonNumber),
                            style = TvType.large,
                            color = Kivo.colors.text,
                            modifier = Modifier.padding(top = Kivo.space(8), bottom = Kivo.space(2)),
                        )
                        when {
                            episodes != null -> EpisodeRows(episodes, head.id, memory, navigator)
                            problem != null -> InlineFailed(problem, LocalSession.current.session.baseUrl, loader::retry)
                            else -> EpisodeRowsSkeleton(model.known(seasonId)?.season?.episodeCount)
                        }
                    }
                }
            }
            if (work != null) extraRows(work, memory, navigator)
        }
    }
    LaunchedEffect(Unit) { memory.restore(if (playable(head, work, choices)) "play" else "favorite") }
}

/** Previous and Next replace the episode page, so Back still goes where it came from. */
private fun Navigator.replaceWork(id: String) = replace(Screen.Work(id))

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
                    HeroText(null, title, collectionCount(count, collection?.source ?: seed?.source), null)
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
