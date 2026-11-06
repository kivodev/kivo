package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.focusGroup
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.DenseListItem
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.api.models.Track
import com.mmcdole.kivo.core.display.pickerName
import com.mmcdole.kivo.core.data.SubtitleSearch
import com.mmcdole.kivo.core.display.TrackName
import com.mmcdole.kivo.core.player.Subtitles

/**
 * A version's audio and subtitle tracks, and which play: a track's ID,
 * [Subtitles.OFF], or null for Automatic; and whether Subtitles offers
 * "Search subtitles…" ([SubtitleSearch.offered]).
 */
data class TrackChoices(
    val audio: List<Track>,
    val subtitles: List<Track>,
    val audioSelected: String?,
    val subtitleSelected: String?,
    val searchable: Boolean = false,
)

/** Whether [TrackChoices] has anything to choose: more than one audio track, a subtitle track besides Off, or a search. */
val TrackChoices.choosable: Boolean get() = audio.size > 1 || subtitles.isNotEmpty() || searchable

/** The track a play starts with when none is chosen: an audio track's ID, and a subtitle track's or [Subtitles.OFF]. */
data class AutomaticTracks(val audio: String?, val subtitle: String)

/**
 * Audio & Subtitles, the panel at the right of the screen that the player's
 * button and the detail page's row open, wide enough for two columns: the
 * Dialogue boost switch, where the play has one, and the audio tracks; and
 * Off and the subtitle tracks. Each track goes by its name ([pickerName]),
 * with the file's own title under it where it says more; the ones chosen
 * are ticked and the others dimmed. Where [TrackChoices.searchable],
 * Subtitles shows even without a track and ends in "Search subtitles…",
 * which calls [onSearch]. Before a play, [automatic] heads each
 * column with Automatic, named after the track it plays, and a choice of it
 * is null. Each column scrolls on its own and opens scrolled to its ticked
 * row; Left and Right move between them, entering one on its ticked row.
 * It opens on the audio, stays open as tracks are chosen, and Back closes it.
 */
@Composable
fun TracksPanel(
    tracks: TrackChoices,
    onAudio: (String?) -> Unit,
    onSubtitle: (String?) -> Unit,
    onDismiss: () -> Unit,
    automatic: AutomaticTracks? = null,
    dialogueBoost: Boolean? = null,
    onDialogueBoost: (Boolean) -> Unit = {},
    onSearch: () -> Unit = {},
) {
    val off = TrackName(stringResource(R.string.off))
    val automaticName = stringResource(R.string.automatic)
    val automaticTrack = stringResource(R.string.automatic_track)
    fun automaticRow(key: String, plays: TrackName?, ticked: Boolean, choose: (String?) -> Unit) =
        TrackRow.Choice(key, null, TrackName(plays?.let { automaticTrack.format(it.name) } ?: automaticName), ticked, choose)

    val audio = buildList {
        if (dialogueBoost != null) add(TrackRow.Boost(dialogueBoost, onDialogueBoost))
        if (tracks.audio.size > 1) {
            if (automatic != null) add(automaticRow("automatic-audio", tracks.audio.firstOrNull { it.id == automatic.audio }?.pickerName, tracks.audioSelected == null, onAudio))
            tracks.audio.forEach { add(TrackRow.Choice(it.id, it.id, it.pickerName, it.id == tracks.audioSelected, onAudio)) }
        }
    }
    val subtitles = buildList {
        if (tracks.subtitles.isNotEmpty() || tracks.searchable) {
            if (automatic != null && tracks.subtitles.isNotEmpty()) {
                val plays = if (automatic.subtitle == Subtitles.OFF) off else tracks.subtitles.firstOrNull { it.id == automatic.subtitle }?.pickerName
                add(automaticRow("automatic-subtitle", plays, tracks.subtitleSelected == null, onSubtitle))
            }
            add(TrackRow.Choice(Subtitles.OFF, Subtitles.OFF, off, tracks.subtitleSelected == Subtitles.OFF, onSubtitle))
            tracks.subtitles.forEach {
                add(TrackRow.Choice(it.id, it.id, it.pickerName, it.id == tracks.subtitleSelected, onSubtitle))
            }
            if (tracks.searchable) add(TrackRow.Search(onSearch))
        }
    }
    val audioTitle = stringResource(R.string.audio)
    val subtitlesTitle = stringResource(R.string.subtitles)
    val start = remember { FocusRequester() }
    SidePanel(stringResource(R.string.audio_and_subtitles), onDismiss, width = PanelWidth) {
        Row(Modifier.weight(1f, fill = false), horizontalArrangement = Arrangement.spacedBy(Kivo.space(6))) {
            if (audio.isNotEmpty()) TrackColumn(audioTitle, audio, start, Modifier.weight(1f).testTag("audio-column"))
            if (subtitles.isNotEmpty()) {
                TrackColumn(subtitlesTitle, subtitles, if (audio.isEmpty()) start else null, Modifier.weight(1f).testTag("subtitle-column"))
            }
        }
    }
    LaunchedEffect(Unit) { focusWhenPlaced(start) }
}

/** A row of Audio & Subtitles: the Dialogue boost switch, a track, Off or Automatic, [ticked] when it's chosen, or Search subtitles. */
private sealed interface TrackRow {
    val key: String
    val ticked: Boolean

    data class Boost(val on: Boolean, val switch: (Boolean) -> Unit) : TrackRow {
        override val key get() = "dialogue-boost"
        override val ticked get() = false
    }

    /** "Search subtitles…", which opens the search. */
    data class Search(val open: () -> Unit) : TrackRow {
        override val key get() = "search-subtitles"
        override val ticked get() = false
    }

    /** A track's ID or [Subtitles.OFF] as [value], or null for Automatic. */
    data class Choice(
        override val key: String,
        val value: String?,
        val name: TrackName,
        override val ticked: Boolean,
        val choose: (String?) -> Unit,
    ) : TrackRow
}

/** The panel's width, enough for two columns of names. */
private val PanelWidth = 640.dp

/**
 * One kind's rows under its [title], scrolled to its ticked row as it
 * opens. Entering the column lands on its ticked row, which [focusFirst],
 * when given, focuses.
 */
@Composable
private fun TrackColumn(title: String, rows: List<TrackRow>, focusFirst: FocusRequester?, modifier: Modifier) {
    val ticked = rows.indexOfFirst { it.ticked }
    val state = rememberLazyListState(initialFirstVisibleItemIndex = (ticked - 2).coerceAtLeast(0))
    val entry = focusFirst ?: remember { FocusRequester() }
    val target = ticked.coerceAtLeast(0)
    Column(modifier) {
        Text(
            title,
            style = TvType.smallStrong,
            color = Kivo.colors.onPlayerMuted,
            modifier = Modifier.padding(start = Kivo.space(3), bottom = Kivo.space(2)),
        )
        KeepInView {
            LazyColumn(
                Modifier
                    .focusProperties { onEnter = { if (ticked >= 0) entry.requestFocus() } }
                    .focusGroup(),
                state = state,
                contentPadding = PaddingValues(bottom = Kivo.space(6)),
            ) {
                items(rows.size, key = { rows[it].key }) { i ->
                    val focus = if (i == target) Modifier.focusRequester(entry) else Modifier
                    when (val row = rows[i]) {
                        is TrackRow.Boost -> BoostRow(row, focus)
                        is TrackRow.Choice -> ChoiceRow(row, focus)
                        is TrackRow.Search -> SearchRow(row, focus)
                    }
                }
            }
        }
    }
}

@Composable
private fun ChoiceRow(row: TrackRow.Choice, modifier: Modifier) {
    val colors = Kivo.colors
    DenseListItem(
        selected = row.ticked,
        onClick = { if (!row.ticked) row.choose(row.value) },
        modifier = modifier.testTag("track:${row.key}"),
        headlineContent = {
            Text(row.name.name, style = if (row.ticked) TvType.bodyStrong else TvType.body, maxLines = 1, overflow = TextOverflow.Ellipsis)
        },
        supportingContent = row.name.note?.let { { Text(it, style = TvType.small, maxLines = 1, overflow = TextOverflow.Ellipsis) } },
        leadingContent = { Box(Modifier.size(24.dp)) { if (row.ticked) KivoIconView(KivoIcon.Check, size = 24.dp) } },
        colors = ListItemStyle.colors(content = if (row.ticked) colors.onPlayer else colors.onPlayerMuted),
        shape = ListItemStyle.shape(),
        scale = ListItemStyle.scale(),
        border = ListItemStyle.border(),
    )
}

/** "Search subtitles…", an action rather than a choice, so never dimmed. */
@Composable
private fun SearchRow(row: TrackRow.Search, modifier: Modifier) {
    val colors = Kivo.colors
    DenseListItem(
        selected = false,
        onClick = row.open,
        modifier = modifier.testTag("track:${row.key}"),
        headlineContent = { Text(stringResource(R.string.search_subtitles), style = TvType.body, maxLines = 1) },
        leadingContent = { Spacer(Modifier.width(24.dp)) },
        colors = ListItemStyle.colors(content = colors.onPlayer),
        shape = ListItemStyle.shape(),
        scale = ListItemStyle.scale(),
        border = ListItemStyle.border(),
    )
}

/** The Dialogue boost switch. */
@Composable
private fun BoostRow(row: TrackRow.Boost, modifier: Modifier) {
    val colors = Kivo.colors
    DenseListItem(
        selected = false,
        onClick = { row.switch(!row.on) },
        modifier = modifier.padding(bottom = Kivo.space(3)).testTag("dialogue-boost"),
        headlineContent = { Text(stringResource(R.string.dialogue_boost), style = TvType.body) },
        leadingContent = { Spacer(Modifier.width(24.dp)) },
        trailingContent = { SettingSwitch(row.on) },
        colors = ListItemStyle.colors(content = colors.onPlayer),
        shape = ListItemStyle.shape(),
        scale = ListItemStyle.scale(),
        border = ListItemStyle.border(),
    )
}
