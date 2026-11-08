package com.mmcdole.kivo.android.mobile.ui

import androidx.compose.foundation.layout.widthIn
import androidx.compose.runtime.Composable
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.Dp
import com.mmcdole.kivo.android.mobile.theme.KivoTokens

/**
 * The window has the wider grids and pages: a tablet's or an unfolded
 * foldable's, medium or expanded in width and not compact in height, which
 * has the navigation rail. A phone, in either orientation, isn't.
 */
val LocalWide = compositionLocalOf { false }

/** In a wide window, at most [width] wide, from the leading edge; in a narrow one, as it is. */
@Composable
fun Modifier.wideWidth(width: Dp = KivoTokens.Container.prose): Modifier = if (LocalWide.current) widthIn(max = width) else this
