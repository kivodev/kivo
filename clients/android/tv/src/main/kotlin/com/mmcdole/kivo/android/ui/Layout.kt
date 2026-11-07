package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.layout
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.offset

// The TV canvas: 960 by 540 dp at 1080p, so a dp is two of the Samsung TV
// app's pixels, whose layout this follows, but with the overscan margins an
// Android TV needs, as many TVs crop the screen's edges, which that app's
// display doesn't: six posters or three landscape cards across a library grid.

/** The margin at the right and bottom of the screen, clear of overscan. */
val SafeEdge = 48.dp

/** The margin at the top of a screen. */
val SafeTop = 27.dp

/** The gap between cards, which leaves a focused card's scale and glow clear of the cards beside it. */
val CardGap = 28.dp

/** Posters across a library grid. */
const val PosterColumns = 6

/** Landscape cards across a library grid. */
const val LandscapeColumns = 3

/** A poster's width in a row. */
val PosterWidth = 110.dp

/** A poster's width in a grid: a column of the library grid beside its letter rail. */
val GridPosterWidth = 104.dp

/** A landscape card's width in a library grid. */
val LandscapeGridWidth = 236.dp

/** A landscape card's width in a row. */
val LandscapeWidth = 250.dp

/** An episode card's width in a season's row: four across, so the row fits under an episode's details. */
val EpisodeCardWidth = 181.dp

fun cardWidth(shape: CardShape) = if (shape == CardShape.Landscape) LandscapeWidth else PosterWidth

/**
 * Room above and below a row's cards. A row's scroll clip lets a further 30 dp past its top and bottom, which leaves
 * 46 dp for the 32.5 dp the glow reaches above a poster; below a card, its caption gives more.
 */
val FocusBleed = 16.dp

/**
 * Room before a row's or a grid's first card for a focused card's glow, which a row clips at its sides with nothing to
 * spare: the glow's blur is drawn 22 dp past the card (20 dp radius, 2 dp spread), 24.2 dp once scaled by 1.1, and the
 * scale moves a 250 dp card's edge out 12.5 dp, so it reaches 36.7 dp.
 */
val RowBleed = 37.dp

/**
 * A row of cards reaching [RowBleed] before where it's put, with room for focus, its first card still lined up with the
 * title above. This only places it, so it costs no layer.
 */
fun Modifier.bleed(): Modifier = layout { measurable, constraints ->
    val room = RowBleed.roundToPx()
    val placeable = measurable.measure(constraints.offset(horizontal = room))
    layout(placeable.width - room, placeable.height) { placeable.place(-room, 0) }
}

/**
 * A card row's padding: room for focus all round, and at its end the screen's margin, for a row that runs to the
 * screen's edge to show there's more while its last card can still come to rest inside the margin.
 */
val RowPadding = PaddingValues(start = RowBleed, top = FocusBleed, end = SafeEdge, bottom = FocusBleed)

/** The gap between a card and its caption: a focused card grows into it without covering the title. */
val CaptionGap = 16.dp

