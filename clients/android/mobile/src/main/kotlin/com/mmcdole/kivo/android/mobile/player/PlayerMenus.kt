package com.mmcdole.kivo.android.mobile.player

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowLeft
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.Icon
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.scale
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.Layout
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.mmcdole.kivo.android.mobile.theme.KivoTokens

/** The player's popups: Audio & Subtitles, and Playback Settings. */
enum class PlayerMenu { Tracks, Settings }

/** The lists a popup's rows open in its place. */
enum class SubMenu { Audio, Subtitles, Quality, Version, Speed }

/**
 * Which popup is open, and the list open in it: Back goes from a list to
 * its popup's rows, and from those closes it.
 */
@Stable
class MenuState {
    var open by mutableStateOf<PlayerMenu?>(null)
        private set
    var sub by mutableStateOf<SubMenu?>(null)
        private set

    fun open(menu: PlayerMenu) {
        open = menu
        sub = null
    }

    fun openSub(sub: SubMenu) {
        this.sub = sub
    }

    fun back() {
        if (sub != null) sub = null else open = null
    }

    fun close() {
        open = null
        sub = null
    }
}

/**
 * One row of a popup: its [label], its [value] at the end and a chevron
 * where it [opens] a list, a tick where it's [ticked] in a list, or a
 * switch where it's a setting that's on or off.
 */
data class MenuItem(
    val id: String,
    val label: String,
    val value: String? = null,
    val opens: Boolean = false,
    val ticked: Boolean = false,
    val switch: Boolean? = null,
    val enabled: Boolean = true,
)

/** What a popup shows: its rows, under a row of [back]'s name that goes back to its popup's rows when it's a list one opened. */
data class MenuPage(val items: List<MenuItem>, val back: String? = null) {
    /** Whether its rows are a list with one ticked, which keeps a column for the tick. */
    val ticks: Boolean get() = back != null
}

/**
 * The popup [menus] has open, standing above the button that opened it
 * ([anchor], in the player's coordinates) with its right edge at the
 * button's, on the player's opaque panel; [width] holds as a list replaces
 * its rows, and it scrolls where it's taller than the room above the
 * button. [pageOf] gives what it shows: a row that opens a list shows that
 * list in its place, under a row of the list's name that goes back, as
 * Back does; any other row goes to [onChoose]. A tap outside closes it.
 */
@Composable
fun PlayerPopup(
    menus: MenuState,
    anchor: Rect,
    width: Dp,
    pageOf: @Composable (PlayerMenu, SubMenu?) -> MenuPage,
    onChoose: (PlayerMenu, SubMenu?, String) -> Unit,
) {
    val menu = menus.open ?: return
    val sub = menus.sub
    BackHandler { menus.back() }
    Box(
        Modifier
            .fillMaxSize()
            .testTag("menuScrim")
            .pointerInput(Unit) { detectTapGestures { menus.close() } },
    )
    val page = pageOf(menu, sub)
    AnchoredAbove(anchor) {
        Column(
            Modifier
                .width(width)
                .clip(PanelShape)
                .background(KivoTokens.dark.playerPanel.copy(alpha = 1f))
                .verticalScroll(rememberScrollState())
                .padding(PanelPadding)
                .testTag("playerMenu"),
        ) {
            key(menu, sub) {
                page.back?.let { back ->
                    PopupRow(back, Modifier.testTag("menuBack"), onClick = menus::back, leading = {
                        Icon(Icons.AutoMirrored.Filled.KeyboardArrowLeft, null, Modifier.size(RowIcon))
                    })
                }
                for (item in page.items) {
                    PopupRow(
                        item.label,
                        Modifier.testTag("menu:${item.id}"),
                        strong = item.ticked,
                        enabled = item.enabled,
                        onClick = {
                            if (item.opens) menus.openSub(SubMenu.valueOf(item.id)) else onChoose(menu, sub, item.id)
                        },
                        leading = if (page.ticks) {
                            { if (item.ticked) Icon(Icons.Default.Check, null, Modifier.size(RowIcon)) }
                        } else {
                            null
                        },
                    ) {
                        item.value?.let {
                            Text(it, style = RowText, color = Muted, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f, fill = false))
                        }
                        if (item.opens) Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, null, Modifier.size(RowIcon), tint = Muted)
                        item.switch?.let { on ->
                            Switch(
                                checked = on,
                                onCheckedChange = null,
                                modifier = Modifier.scale(0.6f).heightIn(max = RowHeight),
                                colors = SwitchDefaults.colors(checkedTrackColor = KivoTokens.dark.accent),
                            )
                        }
                    }
                }
            }
        }
    }
}

/** Places its one child above [anchor], [Lift] clear of it, its right edge at the anchor's but [Margin] inside the screen. */
@Composable
private fun AnchoredAbove(anchor: Rect, content: @Composable () -> Unit) {
    Layout(content, Modifier.fillMaxSize()) { measurables, constraints ->
        val lift = Lift.roundToPx()
        val margin = Margin.roundToPx()
        val room = (anchor.top.toInt() - lift - margin).coerceAtLeast(0)
        val panel = measurables.first().measure(Constraints(maxWidth = constraints.maxWidth - 2 * margin, maxHeight = room))
        layout(constraints.maxWidth, constraints.maxHeight) {
            val x = (anchor.right.toInt() - panel.width).coerceIn(margin, (constraints.maxWidth - panel.width - margin).coerceAtLeast(margin))
            panel.place(x, anchor.top.toInt() - lift - panel.height)
        }
    }
}

/** A popup's row, [RowHeight] high: [leading]'s column where it has one, the [label], and [trailing] at its end. */
@Composable
private fun PopupRow(
    label: String,
    modifier: Modifier = Modifier,
    strong: Boolean = false,
    enabled: Boolean = true,
    onClick: () -> Unit,
    leading: (@Composable () -> Unit)? = null,
    trailing: @Composable androidx.compose.foundation.layout.RowScope.() -> Unit = {},
) {
    Row(
        modifier
            .fillMaxWidth()
            .heightIn(min = RowHeight)
            .clip(RowShape)
            .clickable(enabled = enabled, onClick = onClick)
            .padding(horizontal = RowPadding),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RowGap),
    ) {
        if (leading != null) Box(Modifier.size(RowIcon), contentAlignment = Alignment.Center) { leading() }
        Text(
            label,
            style = if (strong) RowText.copy(fontWeight = FontWeight.SemiBold) else RowText,
            color = if (enabled) KivoTokens.dark.onPlayer else Muted,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f),
        )
        trailing()
    }
}

/** The popup's shape and padding, as the TV apps' player menus have them. */
private val PanelShape = RoundedCornerShape(KivoTokens.Radius.lg)
private val PanelPadding = 6.dp

/** A row's height on one line, a touch target's, its icons, the room at its ends and between its parts. */
private val RowHeight = 48.dp
private val RowIcon = 16.dp
private val RowPadding = 8.dp
private val RowGap = 6.dp
private val RowShape = RoundedCornerShape(KivoTokens.Radius.md)
private val RowText = TextStyle(fontSize = 14.sp, lineHeight = 20.sp)
private val Muted = KivoTokens.dark.onPlayerMuted

/** How far a popup stands clear of its button, and how near the screen's edges it comes. */
private val Lift = 8.dp
private val Margin = 16.dp
