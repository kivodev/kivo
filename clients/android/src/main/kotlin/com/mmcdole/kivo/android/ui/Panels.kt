package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.focusable
import androidx.compose.foundation.gestures.animateScrollBy
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.onClick
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.tracing.trace
import androidx.tv.material3.ListItem
import androidx.tv.material3.RadioButton
import androidx.tv.material3.Switch
import androidx.tv.material3.SwitchDefaults
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import kotlinx.coroutines.launch

/**
 * One choice of a [ChoicePanel], under its [section]'s heading when it has
 * one; a blank name sets a section apart without one. A choice with a
 * [switch] is a setting that is on or off, shown by a switch at its end
 * rather than the panel's marker.
 */
data class Choice(
    val id: String,
    val label: String,
    val detail: String? = null,
    val enabled: Boolean = true,
    val section: String? = null,
    val switch: Boolean? = null,
)

/**
 * A list of choices in a panel at the right of the screen, the current one
 * ticked and focused first, closed by Back. Focus
 * returns to the control that opened it. A panel of switches, such as the
 * filters, ticks each of [ticked] and stays open as they're chosen. A
 * setting's choices ([ChoiceMarker.Radio]) mark the current one with a radio
 * button at the item's end instead of a tick, and a panel of actions
 * ([ChoiceMarker.None]) marks nothing.
 */
@Composable
fun ChoicePanel(
    title: String,
    choices: List<Choice>,
    selected: String?,
    onSelect: (String) -> Unit,
    onDismiss: () -> Unit,
    footer: String? = null,
    ticked: Set<String> = setOfNotNull(selected),
    marker: ChoiceMarker = ChoiceMarker.Tick,
) {
    val colors = Kivo.colors
    SidePanel(title, onDismiss) {
        val initial = remember { FocusRequester() }
        // A list of choices scrolls only as far as the focused one needs.
        KeepInView {
            LazyColumn(Modifier.weight(1f, fill = false), verticalArrangement = Arrangement.spacedBy(Kivo.space(1))) {
                itemsIndexed(choices, key = { _, c -> c.id }) { index, choice ->
                    val isSelected = choice.id in ticked
                    val focusFirst = choice.id == selected || (selected == null || choices.none { it.id == selected }) && index == 0
                    Column {
                        if (choice.section != null && choices.getOrNull(index - 1)?.section != choice.section) {
                            // A section apart; a blank name sets it apart without a heading.
                            if (index > 0) Spacer(Modifier.height(Kivo.space(4)))
                            if (choice.section.isNotBlank()) {
                                Text(
                                    choice.section,
                                    style = TvType.smallStrong,
                                    color = colors.onPlayerMuted,
                                    modifier = Modifier.padding(start = Kivo.space(3), bottom = Kivo.space(1)),
                                )
                            }
                        }
                        ListItem(
                            selected = isSelected,
                            enabled = choice.enabled,
                            onClick = { onSelect(choice.id) },
                            modifier = if (focusFirst) Modifier.focusRequester(initial) else Modifier,
                            headlineContent = { Text(choice.label, style = TvType.bodyStrong) },
                            supportingContent = choice.detail?.let { { Text(it, style = TvType.small) } },
                            leadingContent = if (marker == ChoiceMarker.Tick) {
                                { Box(Modifier.size(20.dp)) { if (isSelected && choice.switch == null) KivoIconView(KivoIcon.Check, size = 20.dp) } }
                            } else {
                                null
                            },
                            trailingContent = when {
                                choice.switch != null -> {
                                    { SettingSwitch(choice.switch, choice.enabled) }
                                }
                                marker == ChoiceMarker.Radio -> {
                                    { RadioButton(selected = isSelected, onClick = null, enabled = choice.enabled) }
                                }
                                else -> null
                            },
                            colors = ListItemStyle.colors(content = colors.onPlayer, disabled = colors.onPlayerMuted),
                            shape = ListItemStyle.shape(),
                            scale = ListItemStyle.scale(),
                            border = ListItemStyle.border(),
                        )
                    }
                }
            }
        }
        if (footer != null) Text(footer, style = TvType.small, color = colors.onPlayerMuted, modifier = Modifier.padding(start = Kivo.space(4), top = Kivo.space(3)))
        LaunchedEffect(Unit) { focusWhenPlaced(initial) }
    }
}

/** A setting's on-or-off switch, flipped by choosing what holds it, so it takes no input of its own. */
@Composable
internal fun SettingSwitch(checked: Boolean, enabled: Boolean = true) {
    val colors = Kivo.colors
    Switch(
        checked = checked,
        onCheckedChange = null,
        enabled = enabled,
        colors = SwitchDefaults.colors(
            checkedThumbColor = colors.onAccent,
            checkedTrackColor = colors.accent,
            uncheckedThumbColor = colors.textMuted,
            uncheckedTrackColor = colors.surfaceRaised,
            uncheckedBorderColor = colors.textMuted,
        ),
    )
}

/**
 * How a [ChoicePanel] marks the chosen: a tick before it, a radio button
 * after it, as a setting's choices are, or nothing, for a panel of actions
 * such as a confirmation.
 */
enum class ChoiceMarker { Tick, Radio, None }

/** What a [SidePanel] is drawn on, where it isn't the usual panel colour: the player's are opaque, as bright video would show through. */
val LocalPanelSurface = staticCompositionLocalOf { Color.Unspecified }

/** A panel at the right of the screen over what opened it, [width] wide, with its title, closed by Back: a list of choices, a long text or a form. */
@Composable
fun SidePanel(
    title: String,
    onDismiss: () -> Unit,
    width: Dp = KivoTokens.Layout.playerPanelWidth,
    content: @Composable ColumnScope.() -> Unit,
) {
    val colors = Kivo.colors
    val surface = LocalPanelSurface.current.takeOrElse { colors.playerPanel }
    // A press that began before the panel opened, such as the OK held to open it, doesn't reach its choices.
    val keys = rememberPanelKeys()
    Dialog(onDismissRequest = onDismiss, properties = panelProperties()) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.CenterEnd) {
            Column(
                Modifier
                    .drawWithContent { trace("Panel:draw") { drawContent() } }
                    .onPreviewKeyEvent { keys.dispatch(it.nativeKeyEvent) }
                    .fillMaxHeight()
                    .width(width)
                    .background(surface)
                    .padding(start = Kivo.space(6), top = SafeTop, end = SafeEdge, bottom = SafeTop),
            ) {
                Text(title, style = TvType.heading, color = colors.text)
                Spacer(Modifier.height(Kivo.space(4)))
                content()
            }
        }
    }
}

/** A long text in full, such as an overview, in a panel; Up and Down scroll it. */
@Composable
fun TextPanel(title: String, text: String, onDismiss: () -> Unit) {
    val scroll = rememberScrollState()
    val scope = rememberCoroutineScope()
    val focus = remember { FocusRequester() }
    val step = with(LocalDensity.current) { 96.dp.toPx() }
    SidePanel(title, onDismiss) {
        Text(
            text,
            style = TvType.body,
            color = Kivo.colors.text,
            modifier = Modifier
                .weight(1f, fill = false)
                .focusRequester(focus)
                .focusable()
                .onKeyEvent { event ->
                    if (event.type != KeyEventType.KeyDown) return@onKeyEvent false
                    when (event.key.nativeKeyCode) {
                        android.view.KeyEvent.KEYCODE_DPAD_DOWN -> scope.launch { scroll.animateScrollBy(step) }
                        android.view.KeyEvent.KEYCODE_DPAD_UP -> scope.launch { scroll.animateScrollBy(-step) }
                        else -> return@onKeyEvent false
                    }
                    true
                }
                .verticalScroll(scroll),
        )
    }
    LaunchedEffect(Unit) { focusWhenPlaced(focus) }
}

/**
 * An overview, its paragraphs run together for the most text in [lines]
 * lines, [textWidth] wide. When it doesn't fit, the whole block, [width]
 * wide, is one stop with a plain outline and a small More at the end of its
 * last line, and OK opens it in full ([TextPanel]), paragraphs and all; when
 * it fits it takes no focus. [onClamped] hears which, for what moves focus
 * around it. Focus shows only in the outline's drawing, so it recomposes
 * nothing.
 */
@Composable
fun Overview(
    title: String,
    text: String,
    modifier: Modifier = Modifier,
    lines: Int = 3,
    width: Dp = 518.dp,
    textWidth: Dp = 470.dp,
    onClamped: (Boolean) -> Unit = {},
) {
    var open by remember { mutableStateOf(false) }
    var clamped by remember(text) { mutableStateOf(false) }
    val focused = remember { mutableStateOf(false) }
    val colors = Kivo.colors
    val shown = remember(text) { oneLine(text) }
    Box(
        modifier
            .width(width)
            .testTag("overview")
            .onFocusChanged { focused.value = it.isFocused }
            .onKeyEvent { event ->
                if (event.key.nativeKeyCode !in OkKeys) return@onKeyEvent false
                if (event.type == KeyEventType.KeyUp) open = true
                true
            }
            .semantics { if (clamped) onClick { open = true; true } }
            .focusable(enabled = clamped)
            .drawBehind {
                if (!focused.value) return@drawBehind
                val across = OverviewOutlineX.toPx()
                val down = OverviewOutlineY.toPx()
                drawRoundRect(
                    colors.focus,
                    topLeft = Offset(-across, -down),
                    size = Size(size.width + across * 2, size.height + down * 2),
                    cornerRadius = CornerRadius(KivoTokens.Radius.md.toPx()),
                    style = Stroke(KivoTokens.Tv.focusRingWidth.toPx()),
                )
            },
    ) {
        Text(
            shown,
            style = TvType.body,
            color = colors.textMuted,
            maxLines = lines,
            minLines = lines,
            overflow = TextOverflow.Ellipsis,
            onTextLayout = {
                if (clamped != it.didOverflowHeight) {
                    clamped = it.didOverflowHeight
                    onClamped(clamped)
                }
            },
            modifier = Modifier.width(textWidth),
        )
        if (clamped) {
            Text(stringResource(R.string.more), style = TvType.smallStrong, color = colors.text, modifier = Modifier.align(Alignment.BottomEnd))
        }
    }
    if (open) TextPanel(title, text) { open = false }
}

/** How far a focused overview's outline stands clear of its text, across and down. */
private val OverviewOutlineX = 10.5.dp
private val OverviewOutlineY = 3.5.dp
