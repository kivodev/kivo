package com.mmcdole.kivo.android.mobile.settings

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.lazy.LazyItemScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowUp
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.R
import com.mmcdole.kivo.android.mobile.home.rowTitle
import com.mmcdole.kivo.android.mobile.app.MobileNav
import com.mmcdole.kivo.android.mobile.library.libraryIcon
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.android.mobile.ui.display
import com.mmcdole.kivo.android.mobile.ui.sentence
import com.mmcdole.kivo.android.settings.SubtitleBackground
import com.mmcdole.kivo.android.settings.SubtitleSize
import com.mmcdole.kivo.api.models.PlaybackPreferences
import com.mmcdole.kivo.api.models.SubtitleMode
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.display.bitrate
import com.mmcdole.kivo.core.display.size
import com.mmcdole.kivo.core.downloads.downloadQuality
import com.mmcdole.kivo.core.settings.DeviceSettings
import com.mmcdole.kivo.core.settings.DownloadQuality
import java.util.Locale

@Composable
private fun preferences(session: ServerSession) = viewModel {
    PreferencesViewModel(session.session, session.graph.core, session.graph.screenSettings)
}

/**
 * A preferences page: [account]'s rows of the account's playback settings,
 * once they're read, under "Your account"; then [device]'s rows of this
 * device's own, under "This device"; and the last change's failure.
 */
@Composable
private fun PreferencesPage(
    title: String,
    tag: String,
    session: ServerSession,
    nav: MobileNav,
    account: (LazyListScope.(PlaybackPreferences, PreferencesViewModel) -> Unit)? = null,
    device: LazyListScope.(DeviceSettings, PreferencesUiState, PreferencesViewModel) -> Unit = { _, _, _ -> },
) {
    val model = preferences(session)
    val state by model.state.collectAsStateWithLifecycle()
    val accountHeading = stringResource(R.string.mobile_your_account)
    val accountNote = stringResource(R.string.mobile_your_account_note)
    val deviceHeading = stringResource(R.string.mobile_this_device)
    val deviceNote = stringResource(R.string.mobile_this_device_note)
    SettingsList(title, nav, tag) {
        if (account != null) {
            item { Label(accountHeading, accountNote) }
            when (val user = state.user) {
                is Load.Loaded -> account(user.value.playback, model)
                is Load.Failed -> item { FailureRow(sentence(user.problem), model::load) }
                Load.Loading -> item { Spinner() }
            }
        }
        state.device?.let { settings ->
            item { Label(deviceHeading, deviceNote) }
            device(settings, state, model)
        }
        state.failure?.let { failure -> item { FailureRow(stringResource(R.string.mobile_couldnt_save, failure), model::retry) } }
    }
}

@Composable
private fun Spinner() {
    Column(Modifier.fillMaxWidth().padding(space(4)), horizontalAlignment = Alignment.CenterHorizontally) { CircularProgressIndicator() }
}

/** Playback: whether up next plays the next episode by itself, and whether a show remembers the tracks last chosen. */
@Composable
fun PlaybackSettingsScreen(session: ServerSession, nav: MobileNav) {
    val autoplay = stringResource(R.string.mobile_autoplay_next)
    val remember = stringResource(R.string.mobile_remember_tracks)
    val rememberNote = stringResource(R.string.mobile_remember_tracks_note)
    PreferencesPage(stringResource(R.string.mobile_settings_playback), "playbackSettings", session, nav, account = { playback, model ->
        item { Toggle(autoplay, playback.autoplayNext, "autoplayNext") { model.setAutoplayNext(it) } }
        item { Toggle(remember, playback.rememberTracks, "rememberTracks", detail = rememberNote) { model.setRememberTracks(it) } }
    })
}

/** Audio: the audio language, on the account; the most channels this device is sent, and dialogue boost. */
@Composable
fun AudioSettingsScreen(session: ServerSession, nav: MobileNav) {
    val languageLabel = stringResource(R.string.mobile_audio_language)
    val languages = listOf(Choice<String?>(null, stringResource(R.string.mobile_original_language))) + languageChoices()
    val channelsLabel = stringResource(R.string.mobile_max_channels)
    val channels = listOf(
        Choice(0, stringResource(R.string.mobile_auto)),
        Choice(2, stringResource(R.string.mobile_stereo)),
        Choice(6, "5.1"),
        Choice(8, "7.1"),
    )
    val channelsNote = stringResource(R.string.mobile_max_channels_note)
    val boost = stringResource(R.string.mobile_dialogue_boost)
    val boostNote = stringResource(R.string.mobile_dialogue_boost_note)
    PreferencesPage(
        stringResource(R.string.mobile_audio),
        "audioSettings",
        session,
        nav,
        account = { playback, model ->
            item { ChoiceRow(languageLabel, withChosen(languages, playback.audioLanguage), playback.audioLanguage, "audioLanguage") { model.setAudioLanguage(it) } }
        },
        device = { settings, _, model ->
            item { ChoiceRow(channelsLabel, channels, settings.audio.maxChannels ?: 0, "maxChannels") { model.setMaxAudioChannels(it) } }
            item { Footnote(channelsNote) }
            item { Toggle(boost, settings.dialogueBoost, "dialogueBoost", detail = boostNote) { model.setDialogueBoost(it) } }
        },
    )
}

/** Subtitles: when they show and in which language, on the account; the size and background of plain ones on this device. */
@Composable
fun SubtitleSettingsScreen(session: ServerSession, nav: MobileNav) {
    val modeLabel = stringResource(R.string.mobile_show_subtitles)
    val modes = listOf(
        Choice(SubtitleMode.None, stringResource(R.string.mobile_off)),
        Choice(SubtitleMode.Forced, stringResource(R.string.mobile_subtitles_forced)),
        Choice(SubtitleMode.Smart, stringResource(R.string.mobile_subtitles_smart)),
        Choice(SubtitleMode.Always, stringResource(R.string.mobile_subtitles_always)),
    )
    val languageLabel = stringResource(R.string.mobile_subtitle_language)
    val languages = listOf(Choice<String?>(null, stringResource(R.string.mobile_server_default))) + languageChoices()
    val sizeLabel = stringResource(R.string.mobile_subtitle_size)
    val sizes = listOf(
        Choice(SubtitleSize.Small, stringResource(R.string.mobile_small)),
        Choice(SubtitleSize.Medium, stringResource(R.string.mobile_medium)),
        Choice(SubtitleSize.Large, stringResource(R.string.mobile_large)),
    )
    val backgroundLabel = stringResource(R.string.mobile_subtitle_background)
    val backgrounds = listOf(
        Choice(SubtitleBackground.None, stringResource(R.string.mobile_background_none)),
        Choice(SubtitleBackground.Shadow, stringResource(R.string.mobile_background_shadow)),
        Choice(SubtitleBackground.Box, stringResource(R.string.mobile_background_box)),
    )
    val styledNote = stringResource(R.string.mobile_styled_subtitles_note)
    PreferencesPage(
        stringResource(R.string.mobile_subtitles),
        "subtitleSettings",
        session,
        nav,
        account = { playback, model ->
            item { ChoiceRow(modeLabel, modes, playback.subtitleMode, "subtitleMode") { model.setSubtitleMode(it) } }
            item {
                ChoiceRow(languageLabel, withChosen(languages, playback.subtitleLanguage), playback.subtitleLanguage, "subtitleLanguage", enabled = playback.subtitleMode != SubtitleMode.None) {
                    model.setSubtitleLanguage(it)
                }
            }
        },
        device = { _, state, model ->
            item { ChoiceRow(sizeLabel, sizes, state.screen.subtitleSize, "subtitleSize") { model.setSubtitleSize(it) } }
            item { ChoiceRow(backgroundLabel, backgrounds, state.screen.subtitleBackground, "subtitleBackground") { model.setSubtitleBackground(it) } }
            item { Footnote(styledNote) }
        },
    )
}

/**
 * Quality: the most a play uses on the server's local network and away from
 * it, from Auto, Original and the server's ladder, kept on this device. A
 * stored rung the ladder doesn't have shows as Auto.
 */
@Composable
fun QualitySettingsScreen(session: ServerSession, nav: MobileNav) {
    val local = stringResource(R.string.mobile_local_quality)
    val remote = stringResource(R.string.mobile_remote_quality)
    PreferencesPage(stringResource(R.string.mobile_quality), "qualitySettings", session, nav, device = { settings, state, model ->
        val qualities = state.qualities
        when {
            qualities.isNotEmpty() -> {
                val choices = qualities.map { Choice(it.quality, qualityName(it)) }
                val rungs = qualities.mapNotNull { it.rung }
                item { ChoiceRow(local, choices, settings.localQuality.onLadder(rungs), "localQuality") { model.setLocalQuality(it) } }
                item { ChoiceRow(remote, choices, settings.remoteQuality.onLadder(rungs), "remoteQuality") { model.setRemoteQuality(it) } }
                state.user.getOrNull()?.limits?.remoteQualityCapBps?.let { cap ->
                    item { RemoteCap(cap) }
                }
            }
            state.qualitiesFailed != null -> item { FailureRow(state.qualitiesFailed, model::load) }
            else -> item { Spinner() }
        }
    })
}

@Composable
private fun RemoteCap(bps: Long) = Footnote(stringResource(R.string.mobile_remote_cap, display.bitrate(bps)))

/**
 * Downloads: the quality videos download at, whether they may use mobile
 * data or wait for Wi-Fi, the space they take, and Delete all downloads,
 * which asks first.
 */
@Composable
fun DownloadSettingsScreen(session: ServerSession, nav: MobileNav) {
    val qualityLabel = stringResource(R.string.mobile_download_quality)
    val qualities = DownloadQuality.entries.map { Choice(it, display.downloadQuality(it.value)) }
    val cellular = stringResource(R.string.mobile_cellular_downloads)
    val cellularNote = stringResource(R.string.mobile_cellular_downloads_note)
    val usedLabel = stringResource(R.string.mobile_storage_used)
    var confirm by rememberSaveable { mutableStateOf(false) }
    val model = preferences(session)
    PreferencesPage(stringResource(R.string.mobile_tab_downloads), "downloadSettings", session, nav, device = { settings, state, model ->
        item { ChoiceRow(qualityLabel, qualities, settings.downloadQuality, "downloadQuality") { model.setDownloadQuality(it) } }
        item { Toggle(cellular, settings.cellularDownloads, "cellularDownloads", detail = cellularNote) { model.setCellularDownloads(it) } }
        state.storage?.let { storage ->
            item { Info(usedLabel, display.size(storage.usedBytes)) }
        }
        item { DeleteAllRow { confirm = true } }
    })
    if (confirm) {
        AlertDialog(
            onDismissRequest = { confirm = false },
            title = { Text(stringResource(R.string.mobile_delete_all_downloads_title)) },
            confirmButton = {
                TextButton(onClick = {
                    confirm = false
                    model.deleteAllDownloads()
                }, Modifier.testTag("confirmDeleteAll")) { Text(stringResource(R.string.mobile_delete), color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { confirm = false }) { Text(stringResource(R.string.mobile_cancel)) } },
        )
    }
}

@Composable
private fun DeleteAllRow(onClick: () -> Unit) = SettingRow(stringResource(R.string.mobile_delete_all_downloads), "deleteAll", danger = true, onClick = onClick)

/**
 * Home & Libraries: Home's rows in order, each with its switch, Move up
 * and Move down, then Reset Home, then every library the user sees, in
 * their order, pinned or not. Pinned ones are in the sidebar; the others
 * are listed under the rest in Libraries.
 */
@Composable
fun LibrariesSettingsScreen(session: ServerSession, nav: MobileNav) {
    val model = viewModel { LibrariesSettingsViewModel(session.session) }
    val state by model.state.collectAsStateWithLifecycle()
    val heading = stringResource(R.string.mobile_settings_home_section)
    val librariesHeading = stringResource(R.string.mobile_tab_libraries)
    val note = stringResource(R.string.mobile_libraries_note)
    SettingsList(stringResource(R.string.mobile_settings_home_libraries), nav, "librariesSettings") {
        item { Label(heading) }
        itemsIndexed(state.rows, key = { _, row -> row.id }) { index, row ->
            OrderedSwitch(
                name = rowTitle(row.kind, row.library?.name),
                checked = row.shown,
                tag = "show:${row.id}",
                canMoveUp = index > 0,
                canMoveDown = index < state.rows.lastIndex,
                onCheck = { model.show(row.id, it) },
                onMove = { model.moveRow(row.id, it) },
            )
        }
        item { SettingRow(stringResource(R.string.mobile_reset_home), "resetHome", onClick = model::resetHome) }
        item { Label(librariesHeading, note) }
        itemsIndexed(state.libraries, key = { _, library -> library.id }) { index, library ->
            OrderedSwitch(
                name = library.name,
                checked = library.pinned,
                tag = "pin:${library.name}",
                canMoveUp = index > 0,
                canMoveDown = index < state.libraries.lastIndex,
                icon = { Icon(libraryIcon(library.kind), null) },
                onCheck = { model.pin(library.id, it) },
                onMove = { model.move(library.id, it) },
            )
        }
        state.failure?.let { item { FailureRow(stringResource(R.string.mobile_couldnt_save, it), model::retry) } }
    }
}

/** A row of Home & Libraries: its name, Move up and Move down, and its switch. */
@Composable
private fun LazyItemScope.OrderedSwitch(
    name: String,
    checked: Boolean,
    tag: String,
    canMoveUp: Boolean,
    canMoveDown: Boolean,
    icon: (@Composable () -> Unit)? = null,
    onCheck: (Boolean) -> Unit,
    onMove: (Int) -> Unit,
) {
    ListItem(
        headlineContent = { Text(name) },
        leadingContent = icon,
        trailingContent = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClick = { onMove(-1) }, enabled = canMoveUp) { Icon(Icons.Default.KeyboardArrowUp, stringResource(R.string.mobile_move_up)) }
                IconButton(onClick = { onMove(1) }, enabled = canMoveDown) { Icon(Icons.Default.KeyboardArrowDown, stringResource(R.string.mobile_move_down)) }
                Switch(checked, onCheckedChange = onCheck, modifier = Modifier.testTag(tag))
            }
        },
        colors = clear(),
        modifier = Modifier.animateItem(),
    )
}

/** Languages by name, from the platform's locale data, as BCP 47 tags. */
@Composable
private fun languageChoices(): List<Choice<String?>> {
    val locale = LocalConfiguration.current.locales[0]
    return remember(locale) {
        Locale.getISOLanguages()
            .map { code -> Choice<String?>(Locale.forLanguageTag(code).toLanguageTag(), languageName(code, locale)) }
            .filter { it.label.isNotBlank() && it.label != it.id }
            .distinctBy { it.label }
            .sortedBy { it.label.lowercase(locale) }
    }
}

/** [choices] with [chosen] among them, named, when it isn't one of them. */
@Composable
private fun withChosen(choices: List<Choice<String?>>, chosen: String?): List<Choice<String?>> {
    if (chosen == null || choices.any { it.id == chosen }) return choices
    val locale = LocalConfiguration.current.locales[0]
    return choices.take(1) + Choice<String?>(chosen, languageName(chosen, locale)) + choices.drop(1)
}

private fun languageName(tag: String, locale: Locale): String =
    Locale.forLanguageTag(tag).getDisplayLanguage(locale).replaceFirstChar { it.titlecase(locale) }
