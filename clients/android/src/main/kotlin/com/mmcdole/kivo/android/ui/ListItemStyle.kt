package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.tv.material3.ListItemDefaults
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens

/**
 * How every list item looks, in a page or a panel: no container until
 * focused, then the hover fill and the focus ring, at its own size, as
 * the other controls show focus. Content dimmed until focused, as a
 * list of tracks dims all but the one playing, takes [focused] then.
 */
object ListItemStyle {
    @Composable
    fun colors(content: Color = Kivo.colors.text, disabled: Color = Kivo.colors.textSubtle, focused: Color = content) = ListItemDefaults.colors(
        containerColor = Color.Transparent,
        contentColor = content,
        focusedContainerColor = Kivo.colors.surfaceHover,
        focusedContentColor = focused,
        pressedContainerColor = Kivo.colors.surfaceHover,
        pressedContentColor = focused,
        selectedContainerColor = Color.Transparent,
        selectedContentColor = content,
        disabledContainerColor = Color.Transparent,
        disabledContentColor = disabled,
        focusedSelectedContainerColor = Kivo.colors.surfaceHover,
        focusedSelectedContentColor = focused,
    )

    @Composable
    fun border() = ListItemDefaults.border(
        focusedBorder = focusRing(),
        focusedSelectedBorder = focusRing(),
        focusedDisabledBorder = focusRing(),
    )

    @Composable
    fun scale() = ListItemDefaults.scale(focusedScale = 1f, focusedSelectedScale = 1f)

    @Composable
    fun shape() = ListItemDefaults.shape(RoundedCornerShape(KivoTokens.Radius.md))
}
