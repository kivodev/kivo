package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.tv.material3.Border
import androidx.tv.material3.ListItemDefaults
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens

/**
 * How every list item looks, in a page or a panel: no container until
 * focused, then lit, white with dark text, at its own size, as the other
 * controls show focus. Content dimmed at rest, as a list of tracks dims all
 * but the one playing, is dark all the same once lit. A disabled item that
 * keeps focus shows it with a ring, as tv-material has no colours for it.
 */
object ListItemStyle {
    @Composable
    fun colors(content: Color = Kivo.colors.text, disabled: Color = Kivo.colors.textSubtle) = ListItemDefaults.colors(
        containerColor = Color.Transparent,
        contentColor = content,
        focusedContainerColor = Lit.container,
        focusedContentColor = Lit.content,
        pressedContainerColor = Lit.container,
        pressedContentColor = Lit.content,
        selectedContainerColor = Color.Transparent,
        selectedContentColor = content,
        disabledContainerColor = Color.Transparent,
        disabledContentColor = disabled,
        focusedSelectedContainerColor = Lit.container,
        focusedSelectedContentColor = Lit.content,
        pressedSelectedContainerColor = Lit.container,
        pressedSelectedContentColor = Lit.content,
    )

    @Composable
    fun border() = ListItemDefaults.border(
        focusedBorder = Border.None,
        focusedSelectedBorder = Border.None,
        focusedDisabledBorder = focusRing(),
    )

    @Composable
    fun scale() = ListItemDefaults.scale(focusedScale = 1f, focusedSelectedScale = 1f)

    @Composable
    fun shape() = ListItemDefaults.shape(RoundedCornerShape(KivoTokens.Radius.md))
}
