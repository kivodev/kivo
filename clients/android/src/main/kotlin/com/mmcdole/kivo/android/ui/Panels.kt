package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.focusable
import androidx.compose.foundation.gestures.animateScrollBy
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.relocation.BringIntoViewRequester
import androidx.compose.foundation.relocation.bringIntoViewRequester
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.takeOrElse
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
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.tracing.trace
import androidx.tv.material3.ClickableSurfaceDefaults
import androidx.tv.material3.ListItem
import androidx.tv.material3.LocalContentColor
import androidx.tv.material3.RadioButton
import androidx.tv.material3.Surface
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
 * rather than the panel's marker. In a menu, a choice's [value] shows at its
 * end, and one that [opens] another list ends in a chevron, and Right opens
 * it as OK does.
 */
data class Choice(
    val id: String,
    val label: String,
    val detail: String? = null,
    val enabled: Boolean = true,
    val section: String? = null,
    val switch: Boolean? = null,
    val value: String? = null,
    val opens: Boolean = false,
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
    SidePanel(title, onDismiss) {
        ChoiceList(choices, selected, onSelect, footer, ticked, marker)
    }
}

/**
 * A [ChoicePanel]'s list, the current choice focused first and scrolled to
 * with two rows above it, for whichever panel holds it. In a [MenuPopup]
 * ([menu]) its rows are a menu's ([MenuRow]), and a list opened from
 * another's row starts with a row of [back]'s name that returns to it, as
 * Left does.
 */
@Composable
fun ColumnScope.ChoiceList(
    choices: List<Choice>,
    selected: String?,
    onSelect: (String) -> Unit,
    footer: String? = null,
    ticked: Set<String> = setOfNotNull(selected),
    marker: ChoiceMarker = ChoiceMarker.Tick,
    menu: Boolean = false,
    back: String? = null,
    onBack: () -> Unit = {},
) {
    val colors = Kivo.colors
    val initial = remember { FocusRequester() }
    val first = choices.indexOfFirst { it.id == selected }.coerceAtLeast(0)
    val above = if (back != null) 1 else 0
    val state = rememberLazyListState(initialFirstVisibleItemIndex = (first + above - 2).coerceAtLeast(0))
    // A list of choices scrolls only as far as the focused one needs.
    KeepInView {
        LazyColumn(
            Modifier
                .weight(1f, fill = false)
                .onKeyEvent { event ->
                    val left = back != null && event.key.nativeKeyCode == android.view.KeyEvent.KEYCODE_DPAD_LEFT
                    if (left && event.type == KeyEventType.KeyDown) onBack()
                    left
                },
            state = state,
            verticalArrangement = Arrangement.spacedBy(if (menu) 0.dp else Kivo.space(1)),
        ) {
            if (back != null) {
                item(key = "back") {
                    MenuRow(back, onBack, Modifier.testTag("menu-back"), leading = { KivoIconView(KivoIcon.ChevronLeft, size = MenuTick) })
                }
            }
            itemsIndexed(choices, key = { _, c -> c.id }) { index, choice ->
                val isSelected = choice.id in ticked
                val focus = if (index == first) Modifier.focusRequester(initial) else Modifier
                if (menu) {
                    MenuChoice(choice, isSelected, marker, focus) { onSelect(choice.id) }
                    return@itemsIndexed
                }
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
                        modifier = focus,
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

/** A [ChoiceList]'s choice in a menu: ticked, or with its value and a chevron where it opens another list, or with its switch. */
@Composable
private fun MenuChoice(choice: Choice, ticked: Boolean, marker: ChoiceMarker, modifier: Modifier, onClick: () -> Unit) {
    MenuRow(
        choice.label,
        onClick,
        modifier.onKeyEvent { event ->
            val right = choice.opens && event.key.nativeKeyCode == android.view.KeyEvent.KEYCODE_DPAD_RIGHT
            if (right && event.type == KeyEventType.KeyDown) onClick()
            right
        },
        detail = choice.detail,
        strong = ticked,
        enabled = choice.enabled,
        leading = if (marker == ChoiceMarker.Tick) {
            { if (ticked && choice.switch == null) KivoIconView(KivoIcon.Check, size = MenuTick) }
        } else {
            null
        },
    ) {
        choice.value?.let { Text(it, style = TvType.small, color = LocalContentColor.current.copy(alpha = MutedAlpha), maxLines = 1) }
        if (choice.opens) KivoIconView(KivoIcon.ChevronRight, size = MenuTick)
        choice.switch?.let { SettingSwitch(it, choice.enabled) }
    }
}

/**
 * A menu's row, [MenuRowHeight] high on one line: its [label], after
 * [leading]'s column where it has one, such as a tick, an optional [detail]
 * line under it, and [trailing] at its end. Focused, it's lit as the other
 * controls are; [dimmed], its content is muted at rest, as a menu of tracks
 * dims the ones not chosen.
 */
@Composable
fun MenuRow(
    label: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    detail: String? = null,
    strong: Boolean = false,
    dimmed: Boolean = false,
    enabled: Boolean = true,
    leading: (@Composable () -> Unit)? = null,
    trailing: @Composable RowScope.() -> Unit = {},
) {
    val colors = Kivo.colors
    val shape = RoundedCornerShape(KivoTokens.Radius.md)
    Surface(
        onClick = onClick,
        enabled = enabled,
        modifier = modifier.fillMaxWidth(),
        shape = ClickableSurfaceDefaults.shape(shape),
        colors = ClickableSurfaceDefaults.colors(
            containerColor = Color.Transparent,
            contentColor = if (dimmed) colors.onPlayerMuted else colors.onPlayer,
            focusedContainerColor = Lit.container,
            focusedContentColor = Lit.content,
            pressedContainerColor = Lit.container,
            pressedContentColor = Lit.content,
            disabledContainerColor = Color.Transparent,
            disabledContentColor = colors.onPlayerMuted,
        ),
        border = ClickableSurfaceDefaults.border(focusedDisabledBorder = focusRing(shape)),
        scale = ClickableSurfaceDefaults.scale(focusedScale = 1f),
    ) {
        Row(
            Modifier
                .heightIn(min = MenuRowHeight)
                .padding(horizontal = MenuRowPadding, vertical = if (detail != null) 5.dp else 7.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(MenuRowGap),
        ) {
            if (leading != null) Box(Modifier.size(MenuTick), contentAlignment = Alignment.Center) { leading() }
            Column(Modifier.weight(1f)) {
                Text(label, style = if (strong) TvType.smallStrong else TvType.small, maxLines = 1, overflow = TextOverflow.Ellipsis)
                if (detail != null) {
                    Text(detail, style = TvType.small, color = LocalContentColor.current.copy(alpha = MutedAlpha), maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
            }
            trailing()
        }
    }
}

/**
 * The width a menu of [lists]' rows takes, each list with its marker's
 * column or none, so the menu keeps it as one list replaces another: as
 * wide as its widest row, from [MenuMinWidth] to [MenuMaxWidth], and the
 * menu's [padding] either side.
 */
@Composable
fun menuWidth(vararg lists: Pair<List<Choice>, ChoiceMarker>, padding: Dp = MenuPadding): Dp {
    val measurer = rememberTextMeasurer()
    val density = LocalDensity.current
    fun text(it: String) = with(density) { measurer.measure(it, TvType.smallStrong, maxLines = 1).size.width.toDp() }
    val widest = lists.maxOfOrNull { (choices, marker) ->
        choices.maxOfOrNull { choice ->
            val leading = if (marker == ChoiceMarker.Tick) MenuTick + MenuRowGap else 0.dp
            val trailing = listOfNotNull(choice.value?.let(::text), MenuTick.takeIf { choice.opens }).fold(0.dp) { sum, it -> sum + MenuRowGap + it }
            MenuRowPadding * 2 + leading + maxOf(text(choice.label), choice.detail?.let(::text) ?: 0.dp) + trailing
        } ?: 0.dp
    } ?: 0.dp
    return widest.coerceIn(MenuMinWidth, MenuMaxWidth) + padding * 2
}

/** A menu's narrowest and widest, inside its padding. */
private val MenuMinWidth = 200.dp
private val MenuMaxWidth = 360.dp

/** A menu row's height on one line, its tick's size, the room at its ends and between its parts. */
private val MenuRowHeight = 30.dp
internal val MenuTick = 10.dp
private val MenuRowPadding = 8.dp
private val MenuRowGap = 6.dp

/** A muted line's alpha over its row's content colour, so it reads on a lit row too. */
private const val MutedAlpha = 0.7f

/**
 * A menu raised over what opened it and no larger than it needs, at [alignment]
 * of the screen less [margin], as the Samsung TV app's Filter and Sort are,
 * and no taller than [maxHeight]; what it holds scrolls inside it. It stands
 * on [LocalPanelSurface] where there is one. Closed by Back, and focus
 * returns to the control that opened it.
 */
@Composable
fun MenuPopup(
    onDismiss: () -> Unit,
    alignment: Alignment,
    margin: PaddingValues,
    width: Dp,
    maxHeight: Dp = Dp.Infinity,
    padding: Dp = MenuPadding,
    content: @Composable ColumnScope.() -> Unit,
) {
    val colors = Kivo.colors
    val surface = LocalPanelSurface.current.takeOrElse { colors.surfaceRaised }
    val keys = rememberPanelKeys()
    val shape = RoundedCornerShape(KivoTokens.Radius.lg)
    Dialog(onDismissRequest = onDismiss, properties = panelProperties()) {
        Box(Modifier.fillMaxSize().padding(margin), contentAlignment = alignment) {
            Column(
                Modifier
                    .onPreviewKeyEvent { keys.dispatch(it.nativeKeyEvent) }
                    .width(width)
                    .heightIn(max = maxHeight)
                    .background(surface, shape)
                    .border(1.dp, colors.border, shape)
                    .padding(padding),
                content = content,
            )
        }
    }
}

/**
 * Where a menu opened from a button stands, as [MenuPopup]'s margin at the
 * screen's bottom end: its bottom [MenuLift] above [rowTop], the top of the
 * button's row, so the row and what's above it stay in view; its end at
 * [end], the button's right edge, but no nearer the screen's edge than
 * [SafeEdge]; and its top no higher than [SafeTop]. Both are from the
 * screen's top left, and [screen] is its size.
 */
fun menuAbove(rowTop: Dp, end: Dp, screen: DpSize): PaddingValues = PaddingValues(
    top = SafeTop,
    end = maxOf(SafeEdge, screen.width - end),
    bottom = (screen.height - rowTop + MenuLift).coerceAtLeast(0.dp),
)

/** How far a menu stands clear of the row of buttons it opens from. */
private val MenuLift = 8.dp

private val MenuPadding = 14.dp
private val ChipGroupGap = 17.dp
private val ChipGap = 7.dp
private val MenuChipHeight = 26.dp

/**
 * [choices] as chips that wrap, grouped under their sections' headings, each of [ticked] outlined,
 * in a menu; it scrolls when they don't all fit, and stays open as they're chosen.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun ColumnScope.ChipChoices(choices: List<Choice>, ticked: Set<String>, onSelect: (String) -> Unit) {
    val colors = Kivo.colors
    val initial = remember { FocusRequester() }
    val scope = rememberCoroutineScope()
    val groups = remember(choices) {
        buildList<MutableList<Choice>> {
            choices.forEach { if (lastOrNull()?.first()?.section == it.section) last() += it else add(mutableListOf(it)) }
        }
    }
    KeepInView {
        Column(
            Modifier.weight(1f, fill = false).verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(ChipGroupGap),
        ) {
            groups.forEach { group ->
                // Moving into a group brings its heading into view along with the chip.
                val requester = remember { BringIntoViewRequester() }
                Column(
                    Modifier
                        .bringIntoViewRequester(requester)
                        .onFocusChanged { if (it.hasFocus) scope.launch { requester.bringIntoView() } },
                    verticalArrangement = Arrangement.spacedBy(ChipGap),
                ) {
                    group.first().section?.takeIf { it.isNotBlank() }?.let {
                        Text(it.uppercase(), style = TvType.smallStrong, color = colors.textMuted)
                    }
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(ChipGap), verticalArrangement = Arrangement.spacedBy(ChipGap)) {
                        group.forEach { choice ->
                            key(choice.id) {
                                Chip(
                                    label = choice.label,
                                    current = choice.id in ticked,
                                    onClick = { onSelect(choice.id) },
                                    modifier = if (choice === choices.first()) Modifier.focusRequester(initial) else Modifier,
                                    height = MenuChipHeight,
                                )
                            }
                        }
                    }
                }
            }
        }
    }
    LaunchedEffect(Unit) { focusWhenPlaced(initial) }
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
