package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.offset
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.mmcdole.kivo.android.theme.KivoTokens

// The TV canvas: 960 by 540 dp at 1080p, with the overscan
// margins TV screens need: six posters or three landscape cards across a
// library grid, and posters the grid's size in every row.

/** The margin at the right and bottom of the screen, clear of overscan. */
val SafeEdge = 48.dp

/** The margin at the top of a screen. */
val SafeTop = 27.dp

/** The gap between cards, as the web grid's. */
val CardGap = KivoTokens.Layout.gridGap

/** Posters across a library grid. */
const val PosterColumns = 6

/** Landscape cards across a library grid. */
const val LandscapeColumns = 3

/** A poster's width, in a grid and in every row: a column of the library grid beside its letter rail. */
val PosterWidth = 116.dp

/** A landscape card's width in a library grid. */
val LandscapeGridWidth = 236.dp

/** A landscape card's width in a row. */
val LandscapeWidth = 252.dp

fun cardWidth(shape: CardShape) = if (shape == CardShape.Landscape) LandscapeWidth else PosterWidth

/** Room at a row's ends for a focused card's scale and ring, which the row would otherwise clip. */
val FocusBleed = 16.dp

/** A row of cards with room for focus at its ends, its first card still lined up with the title above. */
fun Modifier.bleed(): Modifier = offset(x = -FocusBleed)

/** A card row's padding: room for focus all round. */
val RowPadding = PaddingValues(horizontal = FocusBleed, vertical = FocusBleed)

/** The gap between a card and its caption: a focused card grows into it without covering the title. */
val CaptionGap = 16.dp

