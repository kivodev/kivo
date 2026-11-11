package com.mmcdole.kivo.android.settings

import com.mmcdole.kivo.api.models.HomeRowKind
import com.mmcdole.kivo.api.models.LibraryKind
import com.mmcdole.kivo.api.models.ListKind
import com.mmcdole.kivo.api.models.SubtitleMode
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.home.rowTitle
import com.mmcdole.kivo.android.home.tvRows
import com.mmcdole.kivo.android.main.tvLibraries
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.Choice
import com.mmcdole.kivo.android.ui.IconButton
import com.mmcdole.kivo.android.ui.KivoIcon
import com.mmcdole.kivo.android.ui.KivoIconView
import com.mmcdole.kivo.core.Load
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.android.ui.FocusMemory
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import com.mmcdole.kivo.api.models.CurrentUser
import com.mmcdole.kivo.api.models.HomeRowChange
import com.mmcdole.kivo.api.models.HomeRowInfo
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.models.LibraryChoice
import com.mmcdole.kivo.api.models.PlaybackPreferences
import com.mmcdole.kivo.api.models.PlaybackPreferencesUpdate
import com.mmcdole.kivo.core.caps.PassthroughMode

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import java.util.Locale

/** The user's playback preferences, kept on the server, each change sent as it's made. */
class PlaybackViewModel(private val session: ServerSession) : ViewModel() {
    var user by mutableStateOf<Load<CurrentUser>>(Load.Loading)
        private set
    val saves = Saves(viewModelScope)

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            user = try {
                Load.Loaded(session.session.account.currentUser())
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                Load.Failed(RequestProblem.of(e))
            }
        }
    }

    /** Sends [update], made on [row]; the row shows the saved value until the server takes it. */
    fun update(row: String, update: PlaybackPreferencesUpdate) = saves.save(row) {
        user = Load.Loaded(session.session.account.updatePlayback(update))
    }
}

/** Playback: Play next episode automatically and Remember choices per show, on all the user's devices. */
@Composable
internal fun PlaybackPage(session: ServerSession) {
    val model = viewModel { PlaybackViewModel(session) }
    val memory = rememberFocusMemory()
    val state = model.user

    SettingsPage(SettingsGroup.Playback, memory, initial = "autoplay".takeIf { state is Load.Loaded }) {
        accountPart(state, model::load) { playback ->
            item {
                Column {
                    Toggle(
                        stringResource(R.string.autoplay_next),
                        playback.autoplayNext,
                        Modifier.focusKey(memory, "autoplay"),
                    ) { model.update("autoplay", PlaybackPreferencesUpdate(autoplayNext = it)) }
                    SaveFailed(model.saves.on("autoplay"))
                }
            }
            item {
                Column {
                    Toggle(
                        stringResource(R.string.remember_tracks),
                        playback.rememberTracks,
                        Modifier.focusKey(memory, "remember"),
                    ) { model.update("remember", PlaybackPreferencesUpdate(rememberTracks = it)) }
                    SaveFailed(model.saves.on("remember"))
                }
            }
        }
    }
}

/**
 * Audio: the audio language, on all the user's devices; then on this TV
 * passthrough, the most channels and the dialogue boost.
 */
@Composable
internal fun AudioPage(session: ServerSession) {
    val model = viewModel { PlaybackViewModel(session) }
    val device = viewModel { ThisDeviceViewModel(session) }
    val settings by device.settings.collectAsStateWithLifecycle()
    val memory = rememberFocusMemory()
    var picking by remember { mutableStateOf<Picking?>(null) }
    val pick: (Picking) -> Unit = { picking = it }
    val locale = LocalConfiguration.current.locales[0]
    val languages = remember(locale) { languageChoices(locale) }
    val state = model.user

    SettingsPage(SettingsGroup.Audio, memory, initial = "audioLanguage".takeIf { state is Load.Loaded }) {
        accountPart(state, model::load) { playback ->
            item {
                val original = stringResource(R.string.audio_language_original)
                val title = stringResource(R.string.audio_language)
                Column {
                    SettingRow(title, playback.audioLanguage?.let { languageName(it, locale) } ?: original, {
                        pick(Picking(title, listOf(Choice("", original)) + languages, playback.audioLanguage.orEmpty()) {
                            model.update("audioLanguage", PlaybackPreferencesUpdate(audioLanguage = it))
                        })
                    }, Modifier.focusKey(memory, "audioLanguage"))
                    SaveFailed(model.saves.on("audioLanguage"))
                }
            }
        }
        val current = settings ?: return@SettingsPage
        item { ThisTvHeading() }
        item {
            val title = stringResource(R.string.passthrough)
            val choices = listOf(
                Choice(PassthroughMode.Auto.name, stringResource(R.string.passthrough_auto), stringResource(R.string.passthrough_auto_detail)),
                Choice(PassthroughMode.Off.name, stringResource(R.string.passthrough_off), stringResource(R.string.passthrough_off_detail)),
            )
            SettingRow(title, choices.first { it.id == current.audio.passthrough.name }.label, {
                pick(Picking(title, choices, current.audio.passthrough.name) { id ->
                    device.update { it.copy(audio = it.audio.copy(passthrough = PassthroughMode.valueOf(id))) }
                })
            }, Modifier.focusKey(memory, "passthrough"), supporting = stringResource(R.string.passthrough_help))
        }
        item {
            val title = stringResource(R.string.max_audio_channels)
            val choices = listOf(
                Choice("", stringResource(R.string.channels_auto), stringResource(R.string.channels_auto_detail)),
                Choice("2", stringResource(R.string.channels_stereo)),
                Choice("6", "5.1"),
                Choice("8", "7.1"),
            )
            val selected = current.audio.maxChannels?.toString() ?: ""
            SettingRow(title, choices.first { it.id == selected }.label, {
                pick(Picking(title, choices, selected) { id -> device.update { it.copy(audio = it.audio.copy(maxChannels = id.toIntOrNull())) } })
            }, Modifier.focusKey(memory, "maxChannels"))
        }
        item {
            Toggle(stringResource(R.string.dialogue_boost), current.dialogueBoost, Modifier.focusKey(memory, "dialogueBoost"), supporting = stringResource(R.string.dialogue_boost_help)) { on ->
                device.update { it.copy(dialogueBoost = on) }
            }
        }
    }
    picking?.let { PickingPanel(it) { picking = null } }
}

/**
 * Subtitles: Show subtitles and the subtitle language (only while subtitles
 * show), on all the user's devices; then the size and background of
 * subtitles on this TV.
 */
@Composable
internal fun SubtitlesPage(session: ServerSession) {
    val model = viewModel { PlaybackViewModel(session) }
    val device = viewModel { ThisDeviceViewModel(session) }
    val settings by device.screen.collectAsStateWithLifecycle()
    val memory = rememberFocusMemory()
    var picking by remember { mutableStateOf<Picking?>(null) }
    val pick: (Picking) -> Unit = { picking = it }
    val locale = LocalConfiguration.current.locales[0]
    val languages = remember(locale) { languageChoices(locale) }
    val state = model.user

    SettingsPage(SettingsGroup.Subtitles, memory, initial = "subtitleMode".takeIf { state is Load.Loaded }) {
        accountPart(state, model::load) { playback ->
            item {
                val title = stringResource(R.string.show_subtitles)
                val modes = listOf(
                    Choice(SubtitleMode.None.value, stringResource(R.string.subtitles_none)),
                    Choice(SubtitleMode.Forced.value, stringResource(R.string.subtitles_forced), stringResource(R.string.subtitles_forced_detail)),
                    Choice(SubtitleMode.Smart.value, stringResource(R.string.subtitles_smart), stringResource(R.string.subtitles_smart_detail)),
                    Choice(SubtitleMode.Always.value, stringResource(R.string.subtitles_always)),
                )
                Column {
                    SettingRow(title, modes.firstOrNull { it.id == playback.subtitleMode.value }?.label.orEmpty(), {
                        pick(Picking(title, modes, playback.subtitleMode.value) { id ->
                            model.update("subtitleMode", PlaybackPreferencesUpdate(subtitleMode = SubtitleMode.entries.first { it.value == id }))
                        })
                    }, Modifier.focusKey(memory, "subtitleMode"))
                    SaveFailed(model.saves.on("subtitleMode"))
                }
            }
            item {
                val serverDefault = stringResource(R.string.subtitle_language_default)
                val title = stringResource(R.string.subtitle_language)
                Column {
                    SettingRow(title, playback.subtitleLanguage?.let { languageName(it, locale) } ?: serverDefault, {
                        pick(Picking(title, listOf(Choice("", serverDefault)) + languages, playback.subtitleLanguage.orEmpty()) {
                            model.update("subtitleLanguage", PlaybackPreferencesUpdate(subtitleLanguage = it))
                        })
                    }, Modifier.focusKey(memory, "subtitleLanguage"), enabled = playback.subtitleMode != SubtitleMode.None)
                    SaveFailed(model.saves.on("subtitleLanguage"))
                }
            }
        }
        val current = settings ?: return@SettingsPage
        item { ThisTvHeading() }
        item {
            val title = stringResource(R.string.subtitle_size)
            val choices = listOf(
                Choice(SubtitleSize.Small.name, stringResource(R.string.size_small)),
                Choice(SubtitleSize.Medium.name, stringResource(R.string.size_medium)),
                Choice(SubtitleSize.Large.name, stringResource(R.string.size_large)),
            )
            SettingRow(title, choices.first { it.id == current.subtitleSize.name }.label, {
                pick(Picking(title, choices, current.subtitleSize.name) { id -> device.updateScreen { setSubtitleSize(SubtitleSize.valueOf(id)) } })
            }, Modifier.focusKey(memory, "subtitleSize"))
        }
        item {
            val title = stringResource(R.string.subtitle_background)
            val choices = listOf(
                Choice(SubtitleBackground.None.name, stringResource(R.string.background_none)),
                Choice(SubtitleBackground.Shadow.name, stringResource(R.string.background_shadow)),
                Choice(SubtitleBackground.Box.name, stringResource(R.string.background_box)),
            )
            SettingRow(title, choices.first { it.id == current.subtitleBackground.name }.label, {
                pick(Picking(title, choices, current.subtitleBackground.name) { id -> device.updateScreen { setSubtitleBackground(SubtitleBackground.valueOf(id)) } })
            }, Modifier.focusKey(memory, "subtitleBackground"))
        }
    }
    picking?.let { PickingPanel(it) { picking = null } }
}

/**
 * A page's settings of the account, under their heading: [rows] once the
 * user is read ([user]), or why it couldn't be, with Retry ([onRetry]).
 */
private fun LazyListScope.accountPart(user: Load<CurrentUser>, onRetry: () -> Unit, rows: LazyListScope.(PlaybackPreferences) -> Unit) {
    item { AllDevicesHeading() }
    when (user) {
        Load.Loading -> Unit
        is Load.Failed -> item { LoadFailed(user.problem, onRetry) }
        is Load.Loaded -> rows(user.value.playback)
    }
}

/**
 * The user's libraries, kept on the server: every one the user sees, in the
 * user's order, audiobooks too, though the TV lists only the rest. A change
 * shows at once and goes back if the server refuses it.
 */
class LibrariesViewModel(private val session: ServerSession) : ViewModel() {
    var all by mutableStateOf<Load<List<Library>>>(Load.Loading)
        private set
    val saves = Saves(viewModelScope)
    private var reading: Job? = null

    init {
        load()
    }

    /** Reads the libraries, and again as they change; while the server doesn't answer, it asks again. */
    fun load() {
        reading?.cancel()
        all = Load.Loading
        reading = viewModelScope.launch {
            session.session.library.libraries().collect { all = it }
        }
    }

    fun pin(id: String, pinned: Boolean) = change("pin/$id") { libraries -> libraries.map { if (it.id == id) it.copy(pinned = pinned) else it } }

    /** Moves a library up ([by] -1) or down (1) past the next one the TV lists. */
    fun move(id: String, by: Int) = change("move/$id") { moved(it, id, by) }

    private fun change(row: String, transform: (List<Library>) -> List<Library>) {
        val before = all.getOrNull() ?: return
        val after = transform(before)
        all = Load.Loaded(after)
        saves.save(row, undo = { all = Load.Loaded(before) }) {
            session.session.account.setLibraries(after.map { LibraryChoice(it.id, it.pinned) })
        }
    }
}

/**
 * [all] with the library [id] swapped with the next one up ([by] -1) or
 * down (1) among those the TV lists, the others, such as audiobooks
 * libraries, staying where they are; [all] itself when there's none.
 */
internal fun moved(all: List<Library>, id: String, by: Int): List<Library> {
    val listed = tvLibraries(all)
    val from = listed.indexOfFirst { it.id == id }
    if (from < 0) return all
    val other = listed.getOrNull(from + by) ?: return all
    val a = all.indexOfFirst { it.id == id }
    val b = all.indexOfFirst { it.id == other.id }
    return all.toMutableList().apply {
        this[a] = other
        this[b] = listed[from]
    }
}

/**
 * Home's rows, kept on the server: every row the user has, in order, though
 * the TV lists only the rows it shows. A change shows at once and goes back
 * if the server refuses it.
 */
class HomeRowsViewModel(private val session: ServerSession) : ViewModel() {
    var all by mutableStateOf<Load<List<HomeRowInfo>>>(Load.Loading)
        private set
    val saves = Saves(viewModelScope)
    private var reading: Job? = null

    init {
        load()
    }

    /** Reads the layout, and again as it changes; while the server doesn't answer, it asks again. */
    fun load() {
        reading?.cancel()
        all = Load.Loading
        reading = viewModelScope.launch {
            session.session.library.homeRows().collect { all = it }
        }
    }

    fun show(id: String, shown: Boolean) = change("show/$id") { rows -> rows.map { if (it.id == id) it.copy(shown = shown) else it } }

    /** Moves a row up ([by] -1) or down (1) past the next one the TV lists. */
    fun move(id: String, by: Int) = change("rowmove/$id") { movedRow(it, tvHomeRows(it), id, by) }

    fun reset() {
        saves.save("reset") { all = Load.Loaded(session.session.library.resetHomeRows()) }
    }

    private fun change(row: String, transform: (List<HomeRowInfo>) -> List<HomeRowInfo>) {
        val before = all.getOrNull() ?: return
        val after = transform(before)
        all = Load.Loaded(after)
        saves.save(row, undo = { all = Load.Loaded(before) }) {
            session.session.library.setHomeRows(after.map { HomeRowChange(it.id, it.shown) })
        }
    }
}

/** The rows the TV lists of [all]: the kinds Home shows here, and no audiobooks library's. */
internal fun tvHomeRows(all: List<HomeRowInfo>) = all.filter { row ->
    row.kind in tvRows && row.library?.kind != LibraryKind.Audiobooks
}

/**
 * A row of the Home or Libraries page: its switch, and Move up and Move down,
 * which keep focus as it moves, at either end too.
 */
@Composable
private fun OrderedRow(
    name: String,
    checked: Boolean,
    key: String,
    memory: FocusMemory,
    canMoveUp: Boolean,
    canMoveDown: Boolean,
    failed: FailedSave?,
    leading: (@Composable () -> Unit)? = null,
    supporting: String? = null,
    onCheck: (Boolean) -> Unit,
    onMove: (Int) -> Unit,
) {
    Column {
        Row(Modifier.settingWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(Kivo.space(2))) {
            Toggle(name, checked, Modifier.weight(1f).focusKey(memory, "pin/$key"), supporting = supporting, leading = leading, onChange = onCheck)
            IconButton(KivoIcon.ChevronUp, stringResource(R.string.move_up), { onMove(-1) }, Modifier.focusKey(memory, "up/$key"), enabled = canMoveUp)
            IconButton(KivoIcon.ChevronDown, stringResource(R.string.move_down), { onMove(1) }, Modifier.focusKey(memory, "down/$key"), enabled = canMoveDown)
        }
        SaveFailed(failed)
    }
}

/** A row's icon: what kind of row it is, a smart list's in the accent. */
@Composable
private fun HomeRowIcon(row: HomeRowInfo) {
    val icon = when {
        row.list?.kind == ListKind.Smart -> KivoIcon.Sparkle
        row.kind == HomeRowKind.ContinueWatching -> KivoIcon.Play
        row.kind == HomeRowKind.ContinueListening -> KivoIcon.Headphones
        row.kind == HomeRowKind.RecentlyAdded -> KivoIcon.Clock
        else -> KivoIcon.List
    }
    KivoIconView(icon, tint = if (row.list?.kind == ListKind.Smart) Kivo.colors.accent else Kivo.colors.textMuted)
}

/** Home, on all the user's devices: the rows the TV shows, in order, each shown or not, then Reset Home. */
@Composable
internal fun HomePage(session: ServerSession) {
    val rowModel = viewModel { HomeRowsViewModel(session) }
    val memory = rememberFocusMemory()
    val rowState = rowModel.all
    val rows = rowState.getOrNull()?.let(::tvHomeRows)

    SettingsPage(SettingsGroup.Home, memory, initial = rows?.firstOrNull()?.let { "pin/${it.id}" }) {
        item { AllDevicesHeading() }
        if (rowState is Load.Failed) item { LoadFailed(rowState.problem, rowModel::load) }
        if (rows != null) {
            itemsIndexed(rows, key = { _, it -> it.id }) { index, row ->
                OrderedRow(
                    rowTitle(row.kind, row.library?.name, row.list?.name),
                    row.shown,
                    row.id,
                    memory,
                    canMoveUp = index > 0,
                    canMoveDown = index < rows.lastIndex,
                    failed = rowModel.saves.on("show/${row.id}") ?: rowModel.saves.on("rowmove/${row.id}"),
                    leading = { HomeRowIcon(row) },
                    supporting = if (row.broken == true) stringResource(R.string.list_rules_broken) else null,
                    onCheck = { rowModel.show(row.id, it) },
                    onMove = { rowModel.move(row.id, it) },
                )
            }
            item {
                Column {
                    SettingRow(stringResource(R.string.settings_reset_home), "", rowModel::reset, Modifier.focusKey(memory, "reset"))
                    SaveFailed(rowModel.saves.on("reset"))
                }
            }
        }
    }
}

/** Libraries, on all the user's devices: the sidebar's, in the user's order, pinned or not. */
@Composable
internal fun LibrariesPage(session: ServerSession) {
    val libraryModel = viewModel { LibrariesViewModel(session) }
    val memory = rememberFocusMemory()
    val state = libraryModel.all
    val libraries = state.getOrNull()?.let(::tvLibraries)
    val listed = libraries.orEmpty()

    SettingsPage(SettingsGroup.Libraries, memory, initial = libraries?.firstOrNull()?.let { "pin/${it.id}" }) {
        item { AllDevicesHeading() }
        if (state is Load.Failed) item { LoadFailed(state.problem, libraryModel::load) }
        if (libraries != null) {
            if (libraries.isEmpty()) item { Text(stringResource(R.string.no_libraries_yet), style = TvType.body, color = Kivo.colors.text, modifier = Modifier.padding(Kivo.space(4))) }
        }
        itemsIndexed(listed, key = { _, it -> it.id }) { index, library ->
            OrderedRow(
                library.name,
                library.pinned,
                library.id,
                memory,
                canMoveUp = index > 0,
                canMoveDown = index < listed.lastIndex,
                failed = libraryModel.saves.on("pin/${library.id}") ?: libraryModel.saves.on("move/${library.id}"),
                onCheck = { libraryModel.pin(library.id, it) },
                onMove = { libraryModel.move(library.id, it) },
            )
        }
    }
}

/** Languages by name, from the platform's locale data, as BCP 47 tags. */
private fun languageChoices(locale: Locale): List<Choice> =
    Locale.getISOLanguages()
        .map { code -> Choice(Locale.forLanguageTag(code).toLanguageTag(), languageName(code, locale)) }
        .filter { it.label.isNotBlank() && it.label != it.id }
        .distinctBy { it.label }
        .sortedBy { it.label.lowercase(locale) }

private fun languageName(tag: String, locale: Locale): String =
    Locale.forLanguageTag(tag).getDisplayLanguage(locale).replaceFirstChar { it.titlecase(locale) }
