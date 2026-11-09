package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.tv.material3.Border
import androidx.tv.material3.Button
import androidx.tv.material3.ButtonDefaults
import androidx.tv.material3.ClickableSurfaceDefaults
import androidx.tv.material3.Surface
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType

/**
 * A control's label, bold, focused or not, so focus doesn't measure it again.
 * Dark on the white of a lit control it needs the weight: a TV's glare thins
 * dark strokes on white, and semibold in the platform's sans-serif comes out
 * medium, as Roboto has no 600.
 */
val ControlLabel = TvType.bodyStrong.copy(fontWeight = KivoTokens.FontWeights.bold)
private val SmallControlLabel = TvType.smallStrong.copy(fontWeight = KivoTokens.FontWeights.bold)

/** A button: primary (the accent) for a screen's main action, or secondary. Focused, either is lit, the primary's fill dark on the white. */
@Composable
fun KivoButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    icon: KivoIcon? = null,
    primary: Boolean = false,
    enabled: Boolean = true,
    contentPadding: PaddingValues = PaddingValues(horizontal = Kivo.space(4), vertical = Kivo.space(2)),
) {
    val colors = Kivo.colors
    Button(
        onClick = onClick,
        modifier = modifier,
        enabled = enabled,
        shape = ButtonDefaults.shape(RoundedCornerShape(KivoTokens.Radius.md)),
        colors = ButtonDefaults.colors(
            containerColor = if (primary) colors.accent else colors.surfaceRaised,
            contentColor = if (primary) colors.onAccent else colors.text,
            focusedContainerColor = Lit.container,
            focusedContentColor = Lit.content,
            pressedContainerColor = Lit.container,
            pressedContentColor = Lit.content,
            disabledContainerColor = colors.surface,
            disabledContentColor = colors.textSubtle,
        ),
        scale = ButtonDefaults.scale(focusedScale = KivoTokens.Tv.focusScale),
        contentPadding = contentPadding,
    ) {
        if (icon != null) {
            KivoIconView(icon, size = 20.dp)
            if (text.isNotEmpty()) Spacer(Modifier.width(Kivo.space(2)))
        }
        if (text.isNotEmpty()) Text(text, style = ControlLabel, maxLines = 1, overflow = TextOverflow.Ellipsis)
    }
}

/**
 * A round icon button, such as My List or Played, read out as [label] and
 * marked in the accent when on, the light theme's on the white when lit.
 * Disabled, it keeps focus and shows it with a ring, as Move up does once
 * its library reaches the top.
 */
@Composable
fun IconButton(
    icon: KivoIcon,
    label: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    on: Boolean = false,
    enabled: Boolean = true,
    size: Dp = 44.dp,
    corners: Dp = KivoTokens.Radius.full,
    container: Color = Kivo.colors.surfaceRaised,
) {
    val colors = Kivo.colors
    Surface(
        onClick = onClick,
        modifier = modifier.size(size).testTag(label),
        enabled = enabled,
        shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(corners)),
        colors = ClickableSurfaceDefaults.colors(
            containerColor = container,
            contentColor = if (on) colors.accent else colors.text,
            focusedContainerColor = Lit.container,
            focusedContentColor = if (on) KivoTokens.light.accent else Lit.content,
            disabledContainerColor = colors.surface,
            disabledContentColor = colors.textSubtle,
        ),
        scale = ClickableSurfaceDefaults.scale(focusedScale = KivoTokens.Tv.focusScale, focusedDisabledScale = KivoTokens.Tv.focusScale),
        border = ClickableSurfaceDefaults.border(focusedDisabledBorder = focusRing(RoundedCornerShape(KivoTokens.Radius.full))),
    ) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { KivoIconView(icon, size = size / 2, contentDescription = label) }
    }
}

/**
 * A round chip, such as a library's section or a series' season, outlined in
 * the accent when it's the [current] one. Its [trailing] mark reads Kivo.colors
 * in the light theme's while it's lit, so its muted count and accent read on
 * the white.
 */
@Composable
fun Chip(
    label: String,
    current: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    height: Dp = 40.dp,
    trailing: (@Composable () -> Unit)? = null,
) {
    val colors = Kivo.colors
    val interactions = remember { MutableInteractionSource() }
    Surface(
        onClick = onClick,
        modifier = modifier.height(height),
        shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(KivoTokens.Radius.full)),
        colors = ClickableSurfaceDefaults.colors(
            containerColor = if (current) colors.surfaceRaised else Color.Transparent,
            contentColor = colors.text,
            focusedContainerColor = Lit.container,
            focusedContentColor = Lit.content,
        ),
        border = ClickableSurfaceDefaults.border(
            border = Border(BorderStroke(1.dp, if (current) colors.accent else colors.border), shape = RoundedCornerShape(KivoTokens.Radius.full)),
            focusedBorder = Border.None,
        ),
        scale = ClickableSurfaceDefaults.scale(focusedScale = KivoTokens.Tv.focusScale),
        interactionSource = interactions,
    ) {
        Row(Modifier.padding(horizontal = Kivo.space(4)).height(height), verticalAlignment = Alignment.CenterVertically) {
            Text(label, style = SmallControlLabel)
            if (trailing != null) LitOnFocus(interactions, trailing)
        }
    }
}

/** A labelled value that opens a choice when pressed, as the detail page's pickers and the settings rows are. */
@Composable
fun ValueRow(
    label: String,
    value: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    detail: String? = null,
    enabled: Boolean = true,
) {
    val colors = Kivo.colors
    val interactions = remember { MutableInteractionSource() }
    Surface(
        onClick = onClick,
        modifier = modifier,
        enabled = enabled,
        shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(KivoTokens.Radius.md)),
        colors = ClickableSurfaceDefaults.colors(
            containerColor = Color.Transparent,
            contentColor = colors.text,
            focusedContainerColor = Lit.container,
            focusedContentColor = Lit.content,
            disabledContainerColor = Color.Transparent,
            disabledContentColor = colors.textSubtle,
        ),
        scale = ClickableSurfaceDefaults.scale(focusedScale = 1f),
        border = ClickableSurfaceDefaults.border(focusedDisabledBorder = focusRing()),
        interactionSource = interactions,
    ) {
        LitOnFocus(interactions) {
            val shown = Kivo.colors
            Column(Modifier.padding(horizontal = Kivo.space(3), vertical = Kivo.space(2))) {
                Text(label, style = TvType.small, color = shown.textMuted)
                Text(value, style = ControlLabel, maxLines = 1, overflow = TextOverflow.Ellipsis)
                if (detail != null) Text(detail, style = TvType.small, color = shown.textMuted)
            }
        }
    }
}
