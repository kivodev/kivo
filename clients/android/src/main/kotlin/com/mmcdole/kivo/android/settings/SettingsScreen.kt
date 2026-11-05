package com.mmcdole.kivo.android.settings

import androidx.annotation.StringRes
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.tv.material3.ListItem
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.player.systemMatchesFrameRate
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.Choice
import com.mmcdole.kivo.android.ui.ChoiceMarker
import com.mmcdole.kivo.android.ui.ChoicePanel
import com.mmcdole.kivo.android.ui.FocusMemory
import com.mmcdole.kivo.android.ui.KivoButton
import com.mmcdole.kivo.android.ui.KivoIcon
import com.mmcdole.kivo.android.ui.KivoIconView
import com.mmcdole.kivo.android.ui.ListItemStyle
import com.mmcdole.kivo.android.ui.SettingSwitch
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.SafeTop
import com.mmcdole.kivo.android.ui.Screen
import com.mmcdole.kivo.android.ui.display
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.focusWhenPlaced
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.display.problem

/** Settings' groups, in the order Settings lists them, each with its icon. */
enum class SettingsGroup(@param:StringRes val title: Int, val icon: KivoIcon) {
    Account(R.string.settings_account, KivoIcon.Person),
    Playback(R.string.settings_playback, KivoIcon.Play),
    Audio(R.string.audio, KivoIcon.Volume),
    Subtitles(R.string.subtitles, KivoIcon.SpeechBubble),
    Quality(R.string.quality, KivoIcon.Gauge),
    Display(R.string.display, KivoIcon.Tv),
    Libraries(R.string.settings_libraries, KivoIcon.Home),
    About(R.string.settings_about, KivoIcon.Info),
}

/**
 * Settings: a row for each group, each opening the group's page, one level
 * down. Account shows who's signed in.
 */
@Composable
fun SettingsScreen(session: ServerSession, navigator: Navigator) {
    val memory = rememberFocusMemory()
    val user by session.session.user.collectAsStateWithLifecycle()
    SettingsList(stringResource(R.string.settings), memory, initial = SettingsGroup.Account.name) {
        // Display holds frame-rate matching alone, which the system decides from Android 12.
        items(SettingsGroup.entries.filter { it != SettingsGroup.Display || !systemMatchesFrameRate }, key = { it.name }) { group ->
            ActionRow(
                stringResource(group.title),
                { navigator.push(Screen.SettingsPage(group)) },
                Modifier.focusKey(memory, group.name).testTag("settings:${group.name}"),
                supporting = user?.username.takeIf { group == SettingsGroup.Account },
                icon = group.icon,
                opensPage = true,
            )
        }
    }
}

/** One group's page, opened from Settings. */
@Composable
fun SettingsPageScreen(session: ServerSession, group: SettingsGroup, navigator: Navigator) {
    when (group) {
        SettingsGroup.Account -> AccountPage(session)
        SettingsGroup.Playback -> PlaybackPage(session)
        SettingsGroup.Audio -> AudioPage(session)
        SettingsGroup.Subtitles -> SubtitlesPage(session)
        SettingsGroup.Quality -> QualityPage(session)
        SettingsGroup.Display -> DisplayPage(session)
        SettingsGroup.Libraries -> LibrariesPage(session)
        SettingsGroup.About -> AboutPage(session, navigator)
    }
}

/**
 * A group's page: its title, then its rows; a settings page heads its rows
 * with whose they are ([AllDevicesHeading], [ThisTvHeading]). Focus starts
 * on [initial] once the page has it, or returns to the row focused last. A
 * page with nothing to press ([readOnly]), such as About, takes focus as a
 * whole.
 */
@Composable
internal fun SettingsPage(
    group: SettingsGroup,
    memory: FocusMemory,
    initial: String?,
    readOnly: Boolean = false,
    content: LazyListScope.() -> Unit,
) {
    SettingsList(stringResource(group.title), memory, initial, readOnly, content)
}

/** Settings' list: [title] and the rows beside the rail; a setting's choices open in a panel at the right. */
@Composable
internal fun SettingsList(
    title: String,
    memory: FocusMemory,
    initial: String?,
    readOnly: Boolean = false,
    content: LazyListScope.() -> Unit,
) {
    val page = remember { FocusRequester() }
    LazyColumn(
        Modifier
            .fillMaxSize()
            .then(if (readOnly) Modifier.focusRequester(page).focusable() else Modifier),
        contentPadding = PaddingValues(start = 24.dp, top = SafeTop, end = SafeEdge, bottom = SafeEdge),
        verticalArrangement = Arrangement.spacedBy(Kivo.space(1)),
    ) {
        item { PageTitle(title) }
        content()
    }
    LaunchedEffect(initial, readOnly) {
        if (readOnly) focusWhenPlaced(page) else if (initial != null) memory.restore(initial)
    }
}

@Composable
private fun PageTitle(title: String) {
    Text(title, style = TvType.title, color = Kivo.colors.text, modifier = Modifier.padding(start = Kivo.space(4), bottom = Kivo.space(4)))
}

/** A setting's row: the page's width. */
internal fun Modifier.settingWidth(): Modifier = fillMaxWidth()

/** The heading over a page's settings of the account, which follow the user to every device. */
@Composable
internal fun AllDevicesHeading() = SubTitle(stringResource(R.string.scope_all_devices))

/** The heading over a page's settings of this TV, kept on it. */
@Composable
internal fun ThisTvHeading() = SubTitle(stringResource(R.string.this_tv))

/** A headed part of a page. */
@Composable
internal fun SubTitle(title: String) {
    Text(
        title,
        style = TvType.smallStrong,
        color = Kivo.colors.textMuted,
        modifier = Modifier.padding(start = Kivo.space(4), top = Kivo.space(5), bottom = Kivo.space(1)),
    )
}

/**
 * A settings row: a list item with the focus ring every control shows, its
 * [label], an optional line under it ([supporting]), and [trailing] content
 * at its end.
 */
@Composable
private fun SettingItem(
    label: String,
    onClick: () -> Unit,
    modifier: Modifier,
    enabled: Boolean = true,
    supporting: String? = null,
    leading: (@Composable () -> Unit)? = null,
    trailing: (@Composable () -> Unit)? = null,
) {
    val colors = Kivo.colors
    ListItem(
        selected = false,
        onClick = onClick,
        enabled = enabled,
        modifier = modifier.fillMaxWidth(),
        headlineContent = { Text(label, style = TvType.item, maxLines = 1, overflow = TextOverflow.Ellipsis) },
        supportingContent = supporting?.let { { Text(it, style = TvType.small, color = colors.textMuted, maxLines = 1, overflow = TextOverflow.Ellipsis) } },
        leadingContent = leading?.let { { it() } },
        trailingContent = trailing,
        colors = ListItemStyle.colors(),
        shape = ListItemStyle.shape(),
        scale = ListItemStyle.scale(),
        border = ListItemStyle.border(),
    )
}

/**
 * A row that does something when pressed, such as Sign out, or that opens a
 * page ([opensPage]), with a chevron; [supporting] is a line under it, such
 * as who's signed in.
 */
@Composable
internal fun ActionRow(
    label: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    supporting: String? = null,
    icon: KivoIcon? = null,
    opensPage: Boolean = false,
) {
    SettingItem(
        label,
        onClick,
        modifier,
        supporting = supporting,
        leading = icon?.let { { KivoIconView(it, size = 24.dp) } },
        trailing = if (opensPage) {
            { KivoIconView(KivoIcon.ChevronRight, size = 20.dp) }
        } else {
            null
        },
    )
}

/** A setting that is on or off, with a switch at its end, flipped by OK; [supporting] is a line under it. */
@Composable
internal fun Toggle(
    label: String,
    value: Boolean,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    supporting: String? = null,
    onChange: (Boolean) -> Unit,
) {
    SettingItem(
        label,
        { onChange(!value) },
        modifier.testTag("toggle:$label"),
        enabled = enabled,
        supporting = supporting,
        trailing = { SettingSwitch(value, enabled) },
    )
}

/** A fact, such as a version: its label and its value at its end. It takes no focus, as there's nothing to do with it. */
@Composable
internal fun InfoRow(label: String, value: String) {
    SettingItem(
        label,
        {},
        Modifier.focusProperties { canFocus = false },
        trailing = { Text(value, style = TvType.body, color = Kivo.colors.textMuted, maxLines = 1) },
    )
}

/** A setting with its value at its end, which opens its choices; [supporting] is a line under it. */
@Composable
internal fun SettingRow(label: String, value: String, onClick: () -> Unit, modifier: Modifier = Modifier, supporting: String? = null, enabled: Boolean = true) {
    SettingItem(
        label,
        onClick,
        modifier,
        enabled = enabled,
        supporting = supporting,
        trailing = { Text(value, style = TvType.body, color = if (enabled) Kivo.colors.textMuted else Kivo.colors.textSubtle, maxLines = 1) },
    )
}

/** Under a row whose change failed: "Couldn't save." with the reason, and Retry. */
@Composable
internal fun SaveFailed(failed: FailedSave?) {
    failed ?: return
    Row(
        Modifier.settingWidth().padding(horizontal = Kivo.space(4), vertical = Kivo.space(1)),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(Kivo.space(3)),
    ) {
        Text(
            stringResource(R.string.couldnt_save, display.problem(failed.problem)),
            style = TvType.small,
            color = Kivo.colors.danger,
            modifier = Modifier.weight(1f),
        )
        KivoButton(stringResource(R.string.retry), failed.retry, Modifier.testTag("retrySave"))
    }
}

/** A page's read that failed: "Couldn't load this." with the reason, and Retry, which takes focus. */
@Composable
internal fun LoadFailed(problem: RequestProblem, onRetry: () -> Unit) {
    val retry = remember { FocusRequester() }
    Column(Modifier.settingWidth().padding(horizontal = Kivo.space(4)), verticalArrangement = Arrangement.spacedBy(Kivo.space(2))) {
        Text(stringResource(R.string.couldnt_load), style = TvType.bodyStrong, color = Kivo.colors.text)
        Text(display.problem(problem), style = TvType.small, color = Kivo.colors.textMuted)
        Box(Modifier.padding(top = Kivo.space(1))) {
            KivoButton(stringResource(R.string.retry), onRetry, Modifier.focusRequester(retry).testTag("retryLoad"))
        }
    }
    LaunchedEffect(problem) { focusWhenPlaced(retry) }
}

/** A setting being chosen: its title, choices and what picking one does. */
internal class Picking(val title: String, val choices: List<Choice>, val selected: String?, val onPick: (String) -> Unit)

/** The choices of [picking], in a panel at the right, the current one marked, until one is picked or Back closes it. */
@Composable
internal fun PickingPanel(picking: Picking, onClose: () -> Unit) {
    ChoicePanel(
        title = picking.title,
        choices = picking.choices,
        selected = picking.selected,
        marker = ChoiceMarker.Radio,
        onSelect = {
            onClose()
            picking.onPick(it)
        },
        onDismiss = onClose,
    )
}

/** Asks before something that can't be taken back, such as signing out: [title], then Cancel, focused first, or [action]. */
@Composable
internal fun Confirm(title: String, action: String, help: String?, onConfirm: () -> Unit, onDismiss: () -> Unit) {
    ChoicePanel(
        title = title,
        choices = listOf(Choice("cancel", stringResource(R.string.cancel)), Choice("confirm", action)),
        selected = "cancel",
        footer = help,
        ticked = emptySet(),
        marker = ChoiceMarker.None,
        onSelect = {
            onDismiss()
            if (it == "confirm") onConfirm()
        },
        onDismiss = onDismiss,
    )
}
