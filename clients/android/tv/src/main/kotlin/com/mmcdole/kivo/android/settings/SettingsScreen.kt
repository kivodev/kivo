package com.mmcdole.kivo.android.settings

import androidx.activity.compose.BackHandler
import androidx.annotation.StringRes
import androidx.compose.foundation.background
import androidx.compose.foundation.focusGroup
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.runtime.setValue
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.ClickableSurfaceDefaults
import androidx.tv.material3.ListItem
import androidx.tv.material3.Surface
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.player.systemMatchesFrameRate
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.android.ui.Choice
import com.mmcdole.kivo.android.ui.ChoiceMarker
import com.mmcdole.kivo.android.ui.ChoicePanel
import com.mmcdole.kivo.android.ui.FocusMemory
import com.mmcdole.kivo.android.ui.KivoButton
import com.mmcdole.kivo.android.ui.KivoIcon
import com.mmcdole.kivo.android.ui.KivoIconView
import com.mmcdole.kivo.android.ui.ListItemStyle
import com.mmcdole.kivo.android.ui.Lit
import com.mmcdole.kivo.android.ui.LitOnFocus
import com.mmcdole.kivo.android.ui.SettingSwitch
import com.mmcdole.kivo.android.ui.Navigator
import com.mmcdole.kivo.android.ui.SafeEdge
import com.mmcdole.kivo.android.ui.SafeTop
import com.mmcdole.kivo.android.ui.display
import com.mmcdole.kivo.android.ui.focusKey
import com.mmcdole.kivo.android.ui.focusWhenPlaced
import com.mmcdole.kivo.android.ui.rememberFocusMemory
import com.mmcdole.kivo.core.RequestProblem
import com.mmcdole.kivo.core.display.problem

/** Settings' groups, in the order Settings lists them. */
enum class SettingsGroup(@param:StringRes val title: Int) {
    Account(R.string.settings_account),
    Playback(R.string.settings_playback),
    Audio(R.string.audio),
    Subtitles(R.string.subtitles),
    Quality(R.string.quality),
    Display(R.string.display),
    Home(R.string.settings_home_rows),
    Libraries(R.string.settings_libraries),
    About(R.string.settings_about),
}

/** The groups a device lists: Display holds frame-rate matching alone, which the system decides from Android 12. */
internal fun settingsGroups(systemMatches: Boolean): List<SettingsGroup> =
    SettingsGroup.entries.filter { it != SettingsGroup.Display || !systemMatches }

// Settings on the TV canvas, a dp being two of the Samsung TV app's pixels.

/** How far both panes' titles sit from the top of the screen. */
private val PanesTop = 37.dp

/** The sections' column, after the 24 dp every screen starts in from the rail. */
private val SectionsWidth = 210.dp

/** A section's height. */
private val SectionHeight = 35.dp

/** The gap between sections. */
private val SectionGap = 3.5.dp

/** A section's text in from its edge. */
private val SectionInset = 14.dp

/** The gap under the Settings title. */
private val SectionsTitleGap = 14.dp

/** The Settings title's height, [TvType.title]'s line. */
private val SectionsTitleHeight = 30.dp

/** From the sections' column to the line between the panes. */
private val DividerStart = 35.dp

/** From the line between the panes to the settings pane. */
private val PaneStart = 34.5.dp

/** The settings pane's width. */
private val PaneWidth = 543.5.dp

/** The gap under a pane's title, which with the rows' gap puts its first row where the Samsung TV app's card starts. */
private val PaneTitleGap = 10.dp

/**
 * Whether focus belongs in the settings pane, rather than on the sections
 * beside it: only then does a pane take focus for its rows as they arrive.
 * Outside the panes, as in a test of one page, it always may.
 */
private val LocalPaneActive = compositionLocalOf { true }

/** Settings: its sections, and the settings of the one focused there beside them. */
@Composable
fun SettingsScreen(session: ServerSession, navigator: Navigator) {
    val groups = remember { settingsGroups(systemMatchesFrameRate) }
    SettingsPanes(groups) { group ->
        when (group) {
            SettingsGroup.Account -> AccountPage(session)
            SettingsGroup.Playback -> PlaybackPage(session)
            SettingsGroup.Audio -> AudioPage(session)
            SettingsGroup.Subtitles -> SubtitlesPage(session)
            SettingsGroup.Quality -> QualityPage(session)
            SettingsGroup.Display -> DisplayPage(session)
            SettingsGroup.Home -> HomePage(session)
            SettingsGroup.Libraries -> LibrariesPage(session)
            SettingsGroup.About -> AboutPage(session, navigator)
        }
    }
}

/**
 * The sections in a list and, beside them, the [pane] of the one focused
 * there, the only one composed; each keeps its focused row and scroll, and
 * its view models stay with the screen, so coming back to one reads nothing
 * again. Moving along the sections shows each; Right or OK goes into the
 * pane; Left and Back come back to the section, and Back from there goes on
 * to the shell's Back. Up and Down stop at each pane's ends. The first
 * section has focus on arrival, and Back from a page opened in a pane, as
 * Credits is, returns to its row.
 */
@Composable
internal fun SettingsPanes(groups: List<SettingsGroup>, pane: @Composable (SettingsGroup) -> Unit) {
    val colors = Kivo.colors
    val sections = rememberFocusMemory()
    val panes = rememberSaveableStateHolder()
    var shown by rememberSaveable { mutableStateOf(groups.first()) }
    var inPane by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(Unit) { if (!inPane) focusWhenPlaced(sections.requester(shown.name)) }
    BackHandler(enabled = inPane) {
        inPane = false
        sections.requester(shown.name).requestFocus()
    }

    Row(Modifier.fillMaxSize().padding(start = 24.dp)) {
        Column(
            Modifier
                .padding(top = PanesTop)
                .width(SectionsWidth)
                .focusProperties {
                    onEnter = { sections.requester(shown.name).requestFocus() }
                    onExit = {
                        when (requestedFocusDirection) {
                            FocusDirection.Up, FocusDirection.Down -> cancelFocusChange()
                            FocusDirection.Right -> {
                                // The pane takes focus itself, on the row it had, as soon as it has one.
                                cancelFocusChange()
                                inPane = true
                            }
                        }
                    }
                }
                .focusGroup(),
            verticalArrangement = Arrangement.spacedBy(SectionGap),
        ) {
            Text(
                stringResource(R.string.settings),
                style = TvType.title,
                color = colors.text,
                modifier = Modifier.padding(bottom = SectionsTitleGap),
            )
            for (group in groups) {
                SectionEntry(
                    stringResource(group.title),
                    shown = group == shown,
                    onClick = { inPane = true },
                    modifier = Modifier
                        .focusKey(sections, group.name)
                        .onFocusChanged {
                            if (it.isFocused) {
                                shown = group
                                inPane = false
                            }
                        }
                        .testTag("settings:${group.name}"),
                )
            }
        }
        Box(
            Modifier
                .padding(start = DividerStart, top = PanesTop)
                .width(1.dp)
                .height(SectionsTitleHeight + SectionsTitleGap + (SectionHeight + SectionGap) * groups.size)
                .background(colors.border),
        )
        Box(
            Modifier
                .padding(start = PaneStart)
                .width(PaneWidth)
                .fillMaxHeight()
                .focusProperties {
                    onExit = {
                        when (requestedFocusDirection) {
                            FocusDirection.Left -> sections.requester(shown.name).requestFocus()
                            FocusDirection.Up, FocusDirection.Down, FocusDirection.Right -> cancelFocusChange()
                        }
                    }
                }
                .focusGroup(),
        ) {
            CompositionLocalProvider(LocalPaneActive provides inPane) {
                panes.SaveableStateProvider(shown.name) { pane(shown) }
            }
        }
    }
}

/** A section in the list: lit while focused, and marked while its settings are the ones [shown] beside it. */
@Composable
private fun SectionEntry(label: String, shown: Boolean, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val colors = Kivo.colors
    Surface(
        onClick = onClick,
        modifier = modifier.fillMaxWidth().height(SectionHeight),
        shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(KivoTokens.Radius.md)),
        colors = ClickableSurfaceDefaults.colors(
            containerColor = if (shown) colors.surfaceHover else Color.Transparent,
            contentColor = if (shown) colors.text else colors.textMuted,
            focusedContainerColor = Lit.container,
            focusedContentColor = Lit.content,
            pressedContainerColor = Lit.container,
            pressedContentColor = Lit.content,
        ),
        scale = ClickableSurfaceDefaults.scale(focusedScale = 1f),
    ) {
        Box(Modifier.fillMaxSize().padding(start = SectionInset), contentAlignment = Alignment.CenterStart) {
            Text(label, style = TvType.body.copy(fontWeight = KivoTokens.FontWeights.medium), maxLines = 1)
        }
    }
}

/**
 * A group's pane: its title, then its rows; a pane heads its rows with whose
 * they are ([AllDevicesHeading], [ThisTvHeading]). While focus belongs in the
 * pane it goes to [initial] once the pane has it, or returns to the row
 * focused last.
 */
@Composable
internal fun SettingsPage(
    group: SettingsGroup,
    memory: FocusMemory,
    initial: String?,
    content: LazyListScope.() -> Unit,
) {
    val active = LocalPaneActive.current
    LazyColumn(
        Modifier.fillMaxSize(),
        contentPadding = PaddingValues(top = PanesTop, bottom = SafeEdge),
        verticalArrangement = Arrangement.spacedBy(Kivo.space(1)),
    ) {
        item {
            Text(stringResource(group.title), style = TvType.heading, color = Kivo.colors.text, modifier = Modifier.padding(bottom = PaneTitleGap))
        }
        content()
    }
    LaunchedEffect(initial, active) { if (active) memory.restore(initial) }
}

/** A page opened from a pane, such as Credits: [title] and its rows beside the rail; a setting's choices open in a panel at the right. */
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
 * A settings row: a list item, lit when focused as every control is, its
 * [label], an optional line under it ([supporting]), and [trailing] content
 * at its end, all in the light theme's colours while lit.
 */
@Composable
private fun SettingItem(
    label: String,
    onClick: () -> Unit,
    modifier: Modifier,
    enabled: Boolean = true,
    supporting: String? = null,
    trailing: (@Composable () -> Unit)? = null,
) {
    val interactions = remember { MutableInteractionSource() }
    ListItem(
        selected = false,
        onClick = onClick,
        enabled = enabled,
        modifier = modifier.fillMaxWidth(),
        headlineContent = { Text(label, style = TvType.item, maxLines = 1, overflow = TextOverflow.Ellipsis) },
        supportingContent = supporting?.let {
            {
                LitOnFocus(interactions) {
                    Text(it, style = TvType.small, color = Kivo.colors.textMuted, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
            }
        },
        trailingContent = trailing?.let { { LitOnFocus(interactions, it) } },
        colors = ListItemStyle.colors(),
        shape = ListItemStyle.shape(),
        scale = ListItemStyle.scale(),
        border = ListItemStyle.border(),
        interactionSource = interactions,
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
    opensPage: Boolean = false,
) {
    SettingItem(
        label,
        onClick,
        modifier,
        supporting = supporting,
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

/** A page's read that failed: "Couldn't load this." with the reason, and Retry, which takes focus while focus belongs in the pane. */
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
    val active = LocalPaneActive.current
    LaunchedEffect(problem, active) { if (active) focusWhenPlaced(retry) }
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
