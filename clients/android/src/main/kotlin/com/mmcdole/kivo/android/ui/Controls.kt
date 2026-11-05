package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.BorderStroke
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.style.TextOverflow
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

/** A button: primary (the accent) for a screen's main action, or secondary. */
@Composable
fun KivoButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    icon: KivoIcon? = null,
    primary: Boolean = false,
    enabled: Boolean = true,
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
            focusedContainerColor = if (primary) colors.accentHover else colors.surfaceHover,
            focusedContentColor = if (primary) colors.onAccent else colors.text,
            pressedContainerColor = if (primary) colors.accentHover else colors.surfaceHover,
            pressedContentColor = if (primary) colors.onAccent else colors.text,
            disabledContainerColor = colors.surface,
            disabledContentColor = colors.textSubtle,
        ),
        scale = ButtonDefaults.scale(focusedScale = KivoTokens.Tv.focusScale),
        border = ButtonDefaults.border(focusedBorder = focusRing()),
        contentPadding = PaddingValues(horizontal = Kivo.space(4), vertical = Kivo.space(2)),
    ) {
        if (icon != null) {
            KivoIconView(icon, size = 20.dp)
            if (text.isNotEmpty()) Spacer(Modifier.width(Kivo.space(2)))
        }
        if (text.isNotEmpty()) Text(text, style = TvType.bodyStrong, maxLines = 1)
    }
}

/**
 * A round icon button, such as Favorite or Played, read out as [label] and
 * marked in the accent when on. Disabled, it keeps focus and its ring, as
 * Move up does once its library reaches the top.
 */
@Composable
fun IconButton(
    icon: KivoIcon,
    label: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    on: Boolean = false,
    enabled: Boolean = true,
) {
    val colors = Kivo.colors
    Surface(
        onClick = onClick,
        modifier = modifier.size(44.dp).testTag(label),
        enabled = enabled,
        shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(KivoTokens.Radius.full)),
        colors = ClickableSurfaceDefaults.colors(
            containerColor = colors.surfaceRaised,
            contentColor = if (on) colors.accent else colors.text,
            focusedContainerColor = colors.surfaceHover,
            focusedContentColor = if (on) colors.accent else colors.text,
            disabledContainerColor = colors.surface,
            disabledContentColor = colors.textSubtle,
        ),
        scale = ClickableSurfaceDefaults.scale(focusedScale = KivoTokens.Tv.focusScale, focusedDisabledScale = KivoTokens.Tv.focusScale),
        border = ClickableSurfaceDefaults.border(
            focusedBorder = focusRing(RoundedCornerShape(KivoTokens.Radius.full)),
            focusedDisabledBorder = focusRing(RoundedCornerShape(KivoTokens.Radius.full)),
        ),
    ) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { KivoIconView(icon, size = 22.dp, contentDescription = label) }
    }
}

/** A round chip, such as a library's section or a series' season, outlined in the accent when it's the [current] one. */
@Composable
fun Chip(
    label: String,
    current: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    trailing: (@Composable () -> Unit)? = null,
) {
    val colors = Kivo.colors
    Surface(
        onClick = onClick,
        modifier = modifier.height(40.dp),
        shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(KivoTokens.Radius.full)),
        colors = ClickableSurfaceDefaults.colors(
            containerColor = if (current) colors.surfaceRaised else Color.Transparent,
            contentColor = colors.text,
            focusedContainerColor = colors.surfaceHover,
            focusedContentColor = colors.text,
        ),
        border = ClickableSurfaceDefaults.border(
            border = Border(BorderStroke(1.dp, if (current) colors.accent else colors.border), shape = RoundedCornerShape(KivoTokens.Radius.full)),
            focusedBorder = focusRing(RoundedCornerShape(KivoTokens.Radius.full)),
        ),
        scale = ClickableSurfaceDefaults.scale(focusedScale = KivoTokens.Tv.focusScale),
    ) {
        Row(Modifier.padding(horizontal = Kivo.space(4)).height(40.dp), verticalAlignment = Alignment.CenterVertically) {
            Text(label, style = TvType.smallStrong)
            trailing?.invoke()
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
    Surface(
        onClick = onClick,
        modifier = modifier,
        enabled = enabled,
        shape = ClickableSurfaceDefaults.shape(RoundedCornerShape(KivoTokens.Radius.md)),
        colors = ClickableSurfaceDefaults.colors(
            containerColor = Color.Transparent,
            contentColor = colors.text,
            focusedContainerColor = colors.surfaceHover,
            focusedContentColor = colors.text,
            disabledContainerColor = Color.Transparent,
            disabledContentColor = colors.textSubtle,
        ),
        scale = ClickableSurfaceDefaults.scale(focusedScale = 1f),
        border = ClickableSurfaceDefaults.border(focusedBorder = focusRing()),
    ) {
        Column(Modifier.padding(horizontal = Kivo.space(3), vertical = Kivo.space(2))) {
            Text(label, style = TvType.small, color = colors.textMuted)
            Text(value, style = TvType.bodyStrong, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (detail != null) Text(detail, style = TvType.small, color = colors.textMuted)
        }
    }
}
