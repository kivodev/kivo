package com.mmcdole.kivo.android.mobile.detail

import androidx.compose.animation.animateContentSize
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.windowInsetsBottomHeight
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.Favorite
import androidx.compose.material.icons.filled.FavoriteBorder
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Replay
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import kotlinx.coroutines.flow.map
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.app.PersonRoute
import com.mmcdole.kivo.android.mobile.app.PlayerRoute
import com.mmcdole.kivo.android.mobile.downloads.DownloadActions
import com.mmcdole.kivo.android.mobile.downloads.DownloadBadge
import com.mmcdole.kivo.android.mobile.downloads.DownloadButton
import com.mmcdole.kivo.android.mobile.downloads.EpisodesSheet
import com.mmcdole.kivo.android.mobile.downloads.WorkDownloads
import com.mmcdole.kivo.android.mobile.app.handle
import com.mmcdole.kivo.android.mobile.theme.LocalKivo
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.Artwork
import com.mmcdole.kivo.android.mobile.ui.CardCorners
import com.mmcdole.kivo.android.mobile.cast.CastButton
import com.mmcdole.kivo.android.mobile.ui.Failed
import com.mmcdole.kivo.android.mobile.ui.NamedCard
import com.mmcdole.kivo.android.mobile.ui.ProgressBar
import com.mmcdole.kivo.android.mobile.ui.WorkMenu
import com.mmcdole.kivo.android.mobile.ui.WorkMenuHost
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.android.mobile.ui.endsAt
import com.mmcdole.kivo.android.mobile.ui.labelledTitle
import com.mmcdole.kivo.android.mobile.ui.markPlayedLabel
import com.mmcdole.kivo.android.mobile.ui.progressOf
import com.mmcdole.kivo.android.mobile.ui.roleName
import com.mmcdole.kivo.android.mobile.ui.tapAndHold
import com.mmcdole.kivo.api.models.Credit
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.api.models.WorkDetail
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.data.resumeAt
import com.mmcdole.kivo.core.data.selectedVersion
import com.mmcdole.kivo.core.display.clock
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.display.pickerName
import com.mmcdole.kivo.core.display.runtime
import com.mmcdole.kivo.core.display.size
import com.mmcdole.kivo.core.display.timeLeft
import com.mmcdole.kivo.core.downloads.DownloadStatus
import com.mmcdole.kivo.core.player.Subtitles

/**
 * A work's page, as the work's kind has it: a movie, episode, video or
 * book with Play and its tracks, or a series with its seasons and
 * episodes. The artwork runs edge to edge behind the status bar; the top
 * bar takes the page's colour and title once the title scrolls away.
 */
@Composable
fun DetailScreen(session: ServerSession, id: String, seasonId: String?, seasonNumber: Int?, nav: MobileNav) {
    val model = viewModel { DetailViewModel(session.session, id, seasonId, seasonNumber, session.graph.core.settings.map { it.downloadQuality }) }
    val state by model.state.collectAsStateWithLifecycle()
    val head = state.head
    val problem = state.problem
    if (head == null) {
        if (problem != null) {
            Box(Modifier.fillMaxSize().statusBarsPadding()) {
                Failed(problem, stringResource(R.string.mobile_couldnt_load_page), model.serverName, model::retry)
                BackButton(nav)
            }
        } else {
            DetailSkeleton(nav)
        }
        return
    }
    WorkMenuHost({ action, work -> nav.handle(action, work, model.marks) }, seriesPage = head.id.takeIf { head.kind == "series" }) { openMenu ->
        DetailPage(head, state, model, nav, openMenu)
    }
}

@Composable
private fun DetailPage(head: WorkSummary, state: DetailUiState, model: DetailViewModel, nav: MobileNav, openMenu: (WorkMenu) -> Unit) {
    val list = rememberLazyListState()
    val titleShown by remember { derivedStateOf { list.firstVisibleItemIndex > 1 } }
    val seasonHeading = stringResource(R.string.mobile_season_n, head.episode?.seasonNumber ?: 0)
    Box(Modifier.fillMaxSize()) {
        LazyColumn(Modifier.fillMaxSize().testTag("detail"), state = list) {
            item(key = "art") { Backdrop(head) }
            item(key = "hero") { Hero(head, state.work, nav) }
            item(key = "actions") {
                if (head.kind == "series") SeriesActions(head, state, model, nav) else PlayActions(head, state, model, nav)
            }
            if (head.kind != "series") item(key = "tracks") { state.work?.let { TrackRows(it, state.choices, model) } }
            item(key = "overview") { Overview(head.episode?.overview ?: state.work?.overview) }
            when (head.kind) {
                "series" -> seasonItems(state, model, nav, openMenu)
                "episode" -> episodeItems(seasonHeading, state, head.id, nav, openMenu)
            }
            val credits = state.work?.credits.orEmpty()
            if (credits.isNotEmpty()) item(key = "credits") { CreditsRow(credits, nav) }
            item(key = "end") { Spacer(Modifier.windowInsetsBottomHeight(WindowInsets.navigationBars).padding(bottom = space(6))) }
        }
        TopBar(if (titleShown) head.title else null, nav, cast = head.kind != "book")
    }
}

/** The page's top bar over the artwork: back and Cast, then the page's colour and title once the title has scrolled away. */
@Composable
private fun TopBar(title: String?, nav: MobileNav, cast: Boolean) {
    Row(
        Modifier
            .fillMaxWidth()
            .background(if (title != null) MaterialTheme.colorScheme.background else Color.Transparent)
            .statusBarsPadding()
            .height(56.dp)
            .padding(horizontal = space(1)),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        BackButton(nav)
        Text(title.orEmpty(), Modifier.weight(1f), style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
        if (cast) CastButton()
    }
}

@Composable
private fun BackButton(nav: MobileNav) {
    IconButton(
        onClick = nav::back,
        colors = IconButtonDefaults.iconButtonColors(containerColor = LocalKivo.current.badge.copy(alpha = 0.35f)),
    ) {
        Icon(Icons.AutoMirrored.Filled.ArrowBack, stringResource(R.string.mobile_back))
    }
}

/** The artwork, 16:9 under the status bar, fading into the page at its foot. */
@Composable
private fun Backdrop(head: WorkSummary) {
    val image = with(head.images) { if (head.kind == "episode" || head.kind == "video") still ?: backdrop ?: poster else backdrop ?: poster }
    val bg = MaterialTheme.colorScheme.background
    Box(Modifier.fillMaxWidth().aspectRatio(16f / 10f)) {
        Artwork(image, 960.dp, Modifier.matchParentSize())
        Box(Modifier.matchParentSize().background(Brush.verticalGradient(0f to Color.Black.copy(alpha = 0.45f), 0.3f to Color.Transparent, 0.7f to Color.Transparent, 1f to bg)))
    }
}

/** The title, an episode's series over it, and the details line. */
@Composable
private fun Hero(head: WorkSummary, work: WorkDetail?, nav: MobileNav) {
    Column(Modifier.fillMaxWidth().padding(horizontal = space(4)), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(space(1))) {
        head.episode?.let { episode ->
            TextButton(onClick = { nav.openSeries(episode) }) { Text(episode.seriesTitle) }
        }
        Text(
            labelledTitle(head.title, head.movie),
            style = MaterialTheme.typography.headlineMedium,
            textAlign = TextAlign.Center,
            modifier = Modifier.testTag("title"),
        )
        val meta = metaLine(head, work)
        if (meta.isNotEmpty()) Text(meta, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
    }
}

/** The details line: an episode's code, the years, seasons, runtime, rating, content rating and genres. */
@Composable
private fun metaLine(head: WorkSummary, work: WorkDetail?): String {
    val series = head.series
    val years = series?.lastAirYear?.takeIf { it != head.year }?.let { last -> head.year?.let { stringResource(R.string.mobile_years_range, it, last) } }
    return listOfNotNull(
        head.episode?.let { display.episodeCode(it) },
        years ?: head.year?.toString(),
        series?.seasonCount?.let { pluralStringResource(R.plurals.mobile_count_seasons, it, it) },
        head.durationMs?.takeIf { head.kind != "series" }?.let { display.runtime(it.toLong()) },
        head.communityRating?.takeIf { (head.communityVotes ?: 0) > 0 }?.let { stringResource(R.string.mobile_rating, it) },
        head.contentRating,
        work?.genres?.take(2)?.joinToString(", ")?.ifEmpty { null },
    ).joinToString(" · ")
}

/**
 * Play, or Resume with the time and how far in, as a pill across the page;
 * then Play from beginning, Download, Favorite and Watched. "This file is
 * missing" in Play's place when nothing can play.
 */
@Composable
private fun PlayActions(head: WorkSummary, state: DetailUiState, model: DetailViewModel, nav: MobileNav) {
    val work = state.work
    val choices = state.choices
    val available = if (work == null) head.available else work.available && work.selectedVersion(choices.versionId)?.available == true
    val position = if (work != null) work.resumeAt(choices.versionId) else head.userState?.positionMs
    val book = head.kind == "book"
    // A book plays in the book player, from its download when it has one; a video its download when it's ready, with the
    // subtitle chosen here where the download has it, else in the player with the version and tracks chosen here.
    val ready = state.download?.item?.takeIf { it.status == DownloadStatus.Ready }
    fun play(fromStart: Boolean) = when {
        book -> nav.play(head, fromStart)
        ready != null -> nav.play(PlayerRoute(head.id, fromStart, subtitleTrackId = choices.subtitleTrackId, downloadId = ready.id))
        else -> nav.play(head.id, fromStart, choices.versionId, choices.audioTrackId, choices.subtitleTrackId)
    }
    Column(Modifier.padding(horizontal = space(4), vertical = space(4)), verticalArrangement = Arrangement.spacedBy(space(3))) {
        if (available) {
            PlayButton(
                if (position != null && !book) stringResource(R.string.mobile_resume_at, display.clock(position.toLong())) else stringResource(if (position != null) R.string.mobile_resume else R.string.mobile_play),
            ) { play(fromStart = false) }
            val duration = head.durationMs
            if (duration != null) {
                val left = (duration - (position ?: 0)).toLong()
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(space(3))) {
                    progressOf(head)?.let { ProgressBar(it, Modifier.width(72.dp).clip(CircleShape)) }
                    val parts = listOfNotNull(display.timeLeft(left).takeIf { position != null }, endsAt(left).takeIf { head.kind == "movie" || head.kind == "episode" })
                    Text(parts.joinToString(" · "), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        } else {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(space(2))) {
                Icon(Icons.Default.Warning, null, tint = LocalKivo.current.warning)
                Text(stringResource(R.string.mobile_file_missing), color = LocalKivo.current.warning, style = MaterialTheme.typography.titleSmall)
            }
        }
        ActionRow(head, state, model, startOver = if (available && position != null) ({ play(fromStart = true) }) else null)
        state.download?.let { download -> Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) { DownloadBadge(download) } }
    }
}

/** A series' Play: its next episode, or Resume it; its title under. Favorite and Watched beside. */
@Composable
private fun SeriesActions(head: WorkSummary, state: DetailUiState, model: DetailViewModel, nav: MobileNav) {
    val next = state.work?.series?.nextUp
    Column(Modifier.padding(horizontal = space(4), vertical = space(4)), verticalArrangement = Arrangement.spacedBy(space(3))) {
        if (next != null) {
            val position = next.userState?.positionMs
            val episode = next.episode
            PlayButton(
                when {
                    position != null -> stringResource(R.string.mobile_resume_at, display.clock(position.toLong()))
                    episode != null -> stringResource(R.string.mobile_play_episode, episode.seasonNumber, episode.number)
                    else -> stringResource(R.string.mobile_play)
                },
            ) { nav.play(next.id) }
            Text(next.title, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        ActionRow(head, state, model, startOver = next?.takeIf { it.userState?.positionMs != null }?.let { { nav.play(it.id, fromStart = true) } })
    }
}

/** The pill-shaped 48 dp Play button, in the accent. */
@Composable
private fun PlayButton(label: String, onClick: () -> Unit) {
    Button(
        onClick = onClick,
        modifier = Modifier.fillMaxWidth().height(48.dp).testTag("play"),
        shape = CircleShape,
        colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.primary),
    ) {
        Icon(Icons.Default.PlayArrow, null)
        Spacer(Modifier.width(space(2)))
        Text(label, style = MaterialTheme.typography.titleSmall)
    }
}

/**
 * Play from beginning, Download, Favorite and Watched, an icon over a word
 * each; a series' Download offers the season open or the whole series. On a
 * device without downloads there's no Download.
 */
@Composable
private fun ActionRow(head: WorkSummary, state: DetailUiState, model: DetailViewModel, startOver: (() -> Unit)?) {
    val favorite = head.userState?.favorite == true
    val played = head.userState?.played == true
    val download = state.download
    val downloads = model.downloads
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceEvenly) {
        if (head.kind != "series") {
            RoundAction(Icons.Default.Replay, stringResource(R.string.mobile_start_over), "startOver", enabled = startOver != null) { startOver?.invoke() }
            if (download != null && downloads != null && state.work?.available == true) {
                DownloadButton(
                    download,
                    book = head.kind == "book",
                    DownloadActions(downloads::download, { downloads.pause() }, { downloads.resume() }, { downloads.retry() }, { downloads.remove() }, downloads::problemSeen),
                )
            }
        } else if (download != null && downloads != null) {
            SeriesDownload(state, downloads)
        }
        RoundAction(
            if (favorite) Icons.Default.Favorite else Icons.Default.FavoriteBorder,
            stringResource(R.string.mobile_favorite),
            "favorite",
            on = favorite,
        ) { model.setFavorite(head, !favorite) }
        RoundAction(
            if (played) Icons.Default.CheckCircle else Icons.Outlined.CheckCircle,
            stringResource(if (head.kind == "book") R.string.mobile_finished else R.string.mobile_watched),
            "watched",
            on = played,
            description = markPlayedLabel(played, head.kind),
        ) { model.setPlayed(head, !played) }
    }
}

@Composable
private fun RoundAction(
    icon: ImageVector,
    label: String,
    tag: String,
    enabled: Boolean = true,
    on: Boolean = false,
    description: String = label,
    onClick: () -> Unit,
) {
    val color = when {
        !enabled -> MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.38f)
        on -> MaterialTheme.colorScheme.primary
        else -> MaterialTheme.colorScheme.onSurface
    }
    Column(
        Modifier
            .widthIn(min = 72.dp)
            .clip(MaterialTheme.shapes.medium)
            .clickable(enabled = enabled, onClick = onClick)
            .padding(vertical = space(2))
            .testTag(tag),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Icon(icon, description, tint = color)
        Text(label, style = MaterialTheme.typography.labelMedium, color = color, modifier = Modifier.padding(top = space(1)))
    }
}

/** A series' Download: the season open, or the whole series, each opening the episodes sheet. */
@Composable
private fun SeriesDownload(state: DetailUiState, downloads: WorkDownloads) {
    var open by remember { mutableStateOf(false) }
    val seasons = state.seasons.orEmpty()
    Box {
        RoundAction(Icons.Default.Download, stringResource(R.string.mobile_download), "download", enabled = seasons.isNotEmpty()) { open = true }
        DropdownMenu(open, onDismissRequest = { open = false }) {
            state.season?.let { season ->
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.mobile_download_named, season.title)) },
                    onClick = {
                        open = false
                        downloads.chooseEpisodes(season, seasons)
                    },
                    modifier = Modifier.testTag("downloadSeason"),
                )
            }
            DropdownMenuItem(
                text = { Text(stringResource(R.string.mobile_download_series)) },
                onClick = {
                    open = false
                    downloads.chooseEpisodes(null, seasons)
                },
                modifier = Modifier.testTag("downloadSeries"),
            )
        }
    }
    state.download?.let { download ->
        EpisodesSheet(download, onPick = downloads::downloadEpisodes, onDismiss = downloads::closeEpisodes)
        download.problem?.let { problem ->
            AlertDialog(
                onDismissRequest = downloads::problemSeen,
                text = { Text(problem) },
                confirmButton = { TextButton(onClick = downloads::problemSeen) { Text(stringResource(R.string.mobile_ok)) } },
            )
        }
    }
}

/** Which picker sheet is open. */
private enum class Picker { Version, Audio, Subtitles }

/**
 * Version when there's more than one, Audio when there's more than one
 * track, and Subtitles, each a row opening its picker, named by the
 * server's track names; Automatic names the track a play starts with.
 */
@Composable
private fun TrackRows(work: WorkDetail, choices: TrackChoices, model: DetailViewModel) {
    if (!work.available) return
    val version = work.selectedVersion(choices.versionId) ?: return
    val audio = version.tracks.filter { it.type == TrackType.Audio }
    val subtitles = version.tracks.filter { it.type == TrackType.Subtitle }
    var open by rememberSaveable { mutableStateOf<Picker?>(null) }
    val automatic = stringResource(R.string.mobile_automatic)
    val off = stringResource(R.string.mobile_off)
    fun audioName(id: String?) = audio.firstOrNull { it.id == id }?.name
    fun subtitleName(id: String?) = if (id == Subtitles.OFF) off else subtitles.firstOrNull { it.id == id }?.name
    Column(Modifier.padding(bottom = space(2))) {
        if (work.versions.size > 1) TrackRow(stringResource(R.string.mobile_version), version.name, "version") { open = Picker.Version }
        if (audio.size > 1) {
            TrackRow(stringResource(R.string.mobile_audio), audioName(choices.audioTrackId) ?: automatic, "audio") { open = Picker.Audio }
        }
        if (subtitles.isNotEmpty()) {
            TrackRow(stringResource(R.string.mobile_subtitles), subtitleName(choices.subtitleTrackId) ?: automatic, "subtitles") { open = Picker.Subtitles }
        }
    }
    when (open) {
        Picker.Version -> PickerSheet(
            stringResource(R.string.mobile_version),
            work.versions.map { v -> Choice(v.id, v.name, display.size(v.sizeBytes)) },
            version.id,
            onPick = { it?.let(model::chooseVersion) },
            onDismiss = { open = null },
        )
        Picker.Audio -> PickerSheet(
            stringResource(R.string.mobile_audio),
            listOf(Choice(null, stringResource(R.string.mobile_automatic_named, audioName(version.audioTrackId).orEmpty()))) +
                audio.map { Choice(it.id, it.pickerName.name, it.pickerName.note) },
            choices.audioTrackId,
            onPick = model::chooseAudio,
            onDismiss = { open = null },
        )
        Picker.Subtitles -> PickerSheet(
            stringResource(R.string.mobile_subtitles),
            listOf(Choice(null, stringResource(R.string.mobile_automatic_named, subtitleName(version.subtitleTrackId ?: Subtitles.OFF).orEmpty())), Choice(Subtitles.OFF, off)) +
                subtitles.map { Choice(it.id, it.pickerName.name, it.pickerName.note) },
            choices.subtitleTrackId,
            onPick = model::chooseSubtitle,
            onDismiss = { open = null },
        )
        null -> Unit
    }
}

@Composable
private fun TrackRow(label: String, value: String, tag: String, onClick: () -> Unit) {
    ListItem(
        headlineContent = { Text(label) },
        trailingContent = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(value, Modifier.widthIn(max = 220.dp), color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Icon(Icons.Default.ArrowDropDown, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        },
        colors = ListItemDefaults.colors(containerColor = Color.Transparent),
        modifier = Modifier.testTag(tag).clickable(onClick = onClick),
    )
}

/** One of a picker's choices: its ID (null for the server's choice), name and the line under it. */
private data class Choice(val id: String?, val name: String, val note: String? = null)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun PickerSheet(title: String, choices: List<Choice>, selected: String?, onPick: (String?) -> Unit, onDismiss: () -> Unit) {
    ModalBottomSheet(onDismissRequest = onDismiss, modifier = Modifier.testTag("picker")) {
        Text(title, Modifier.padding(horizontal = space(6), vertical = space(2)), style = MaterialTheme.typography.titleMedium)
        LazyColumn(contentPadding = PaddingValues(bottom = space(6))) {
            items(choices) { choice ->
                ListItem(
                    headlineContent = { Text(choice.name) },
                    supportingContent = choice.note?.let { { Text(it) } },
                    trailingContent = { if (choice.id == selected) Icon(Icons.Default.Check, null, tint = MaterialTheme.colorScheme.primary) },
                    colors = ListItemDefaults.colors(containerColor = Color.Transparent),
                    modifier = Modifier.clickable {
                        onPick(choice.id)
                        onDismiss()
                    },
                )
            }
        }
    }
}

/** The overview, three lines until More. */
@Composable
private fun Overview(text: String?) {
    if (text.isNullOrBlank()) return
    var expanded by rememberSaveable { mutableStateOf(false) }
    Column(Modifier.padding(horizontal = space(4), vertical = space(2)).animateContentSize()) {
        Text(text, style = MaterialTheme.typography.bodyMedium, maxLines = if (expanded) Int.MAX_VALUE else 3, overflow = TextOverflow.Ellipsis)
        TextButton(onClick = { expanded = !expanded }, contentPadding = PaddingValues(0.dp)) {
            Text(stringResource(if (expanded) R.string.mobile_less else R.string.mobile_more))
        }
    }
}

/** A series' season picker, "Season 1 ▾", then the open season's episodes. */
private fun LazyListScope.seasonItems(state: DetailUiState, model: DetailViewModel, nav: MobileNav, openMenu: (WorkMenu) -> Unit) {
    val seasons = state.seasons ?: return
    val season = state.season
    item(key = "seasons") {
        var open by remember { mutableStateOf(false) }
        Row(Modifier.padding(horizontal = space(2), vertical = space(2)), verticalAlignment = Alignment.CenterVertically) {
            TextButton(onClick = { open = true }, enabled = seasons.size > 1, modifier = Modifier.testTag("seasonPicker")) {
                Text(season?.title.orEmpty(), style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.onSurface)
                if (seasons.size > 1) Icon(Icons.Default.ArrowDropDown, null)
            }
            Spacer(Modifier.weight(1f))
            season?.season?.episodeCount?.let {
                Text(pluralStringResource(R.plurals.mobile_count_episodes, it, it), Modifier.padding(end = space(2)), color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        if (open) {
            PickerSheet(
                stringResource(R.string.mobile_seasons),
                seasons.map { s ->
                    val unplayed = s.season?.unplayedCount ?: 0
                    Choice(s.id, s.title, if (unplayed > 0) pluralStringResource(R.plurals.mobile_n_unwatched, unplayed, unplayed) else stringResource(R.string.mobile_watched))
                },
                season?.id,
                onPick = { it?.let(model::chooseSeason) },
                onDismiss = { open = false },
            )
        }
    }
    episodeRows(state, null, nav, openMenu)
}

/** An episode's season's episodes, under [heading]. */
private fun LazyListScope.episodeItems(heading: String, state: DetailUiState, current: String, nav: MobileNav, openMenu: (WorkMenu) -> Unit) {
    item(key = "seasonHeading") {
        HorizontalDivider(Modifier.padding(top = space(4)))
        Text(heading, Modifier.padding(horizontal = space(4), vertical = space(3)), style = MaterialTheme.typography.titleMedium)
    }
    episodeRows(state, current, nav, openMenu)
}

private fun LazyListScope.episodeRows(state: DetailUiState, current: String?, nav: MobileNav, openMenu: (WorkMenu) -> Unit) {
    when (val episodes = state.episodes) {
        is Load.Loaded -> items(episodes.value, key = { "e" + it.id }, contentType = { "episode" }) { episode ->
            EpisodeRow(episode, episode.id == current, { nav.openWork(episode) }, { openMenu(WorkMenu(episode)) })
        }
        is Load.Failed -> item(key = "episodesFailed") {
            Text(stringResource(R.string.mobile_couldnt_load_season), Modifier.padding(space(4)), color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        else -> items(3, key = { "skeleton$it" }) { EpisodeSkeleton() }
    }
}

/** An episode: its frame with progress, its number and title, time, and synopsis. */
@Composable
private fun EpisodeRow(episode: WorkSummary, current: Boolean, onClick: () -> Unit, onLongClick: () -> Unit) {
    Column(
        Modifier
            .fillMaxWidth()
            .tapAndHold(onClick, onLongClick)
            .padding(horizontal = space(4), vertical = space(2))
            .testTag("episode:${episode.title}"),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(space(3))) {
            Box(Modifier.width(128.dp).aspectRatio(16f / 9f).clip(CardCorners)) {
                Artwork(episode.images.still ?: episode.images.backdrop, 128.dp, Modifier.matchParentSize())
                progressOf(episode)?.let { ProgressBar(it, Modifier.align(Alignment.BottomCenter)) }
            }
            Column(Modifier.weight(1f)) {
                val number = episode.episode?.let { e -> e.endNumber?.takeIf { it != e.number }?.let { "${e.number}–$it" } ?: e.number.toString() }
                Text(
                    listOfNotNull(number, episode.title).joinToString(". "),
                    style = MaterialTheme.typography.titleSmall,
                    color = if (current) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurface,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                )
                val position = episode.userState?.positionMs
                val duration = episode.durationMs
                val time = when {
                    duration != null && position != null -> display.timeLeft((duration - position).toLong())
                    duration != null -> display.runtime(duration.toLong())
                    else -> null
                }
                time?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
            }
            if (episode.userState?.played == true) Icon(Icons.Default.CheckCircle, stringResource(R.string.mobile_watched), tint = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        episode.episode?.overview?.let {
            Text(it, Modifier.padding(top = space(2)), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 3, overflow = TextOverflow.Ellipsis)
        }
    }
}

@Composable
private fun EpisodeSkeleton() {
    Row(Modifier.padding(horizontal = space(4), vertical = space(2)), horizontalArrangement = Arrangement.spacedBy(space(3))) {
        Box(Modifier.width(128.dp).aspectRatio(16f / 9f).clip(CardCorners).background(LocalKivo.current.placeholder))
        Box(Modifier.width(160.dp).height(16.dp).clip(CardCorners).background(LocalKivo.current.placeholder))
    }
}

/** Cast & Crew: round photos with the name and the character or role, each opening the person's page. */
@Composable
private fun CreditsRow(credits: List<Credit>, nav: MobileNav) {
    Column(Modifier.padding(top = space(4))) {
        Text(stringResource(R.string.mobile_cast_and_crew), Modifier.padding(horizontal = space(4), vertical = space(2)), style = MaterialTheme.typography.titleMedium)
        LazyRow(contentPadding = PaddingValues(horizontal = space(4)), horizontalArrangement = Arrangement.spacedBy(space(3))) {
            items(credits, key = { it.personId + it.role + it.character.orEmpty() }) { credit ->
                NamedCard(credit.name, credit.character ?: roleName(credit.role), credit.photo, 80.dp, round = true, { nav.open(PersonRoute(credit.personId)) })
            }
        }
    }
}

/** A page while its work is read, with nothing known of it yet. */
@Composable
private fun DetailSkeleton(nav: MobileNav) {
    val placeholder = LocalKivo.current.placeholder
    Box(Modifier.fillMaxSize()) {
        Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(space(3))) {
            Box(Modifier.fillMaxWidth().aspectRatio(16f / 10f).background(placeholder))
            Box(Modifier.width(220.dp).height(28.dp).clip(CardCorners).background(placeholder))
            Box(Modifier.width(160.dp).height(16.dp).clip(CardCorners).background(placeholder))
            Box(Modifier.padding(horizontal = space(4)).fillMaxWidth().height(48.dp).clip(CircleShape).background(placeholder))
        }
        Box(Modifier.statusBarsPadding().padding(space(1))) { BackButton(nav) }
    }
}
