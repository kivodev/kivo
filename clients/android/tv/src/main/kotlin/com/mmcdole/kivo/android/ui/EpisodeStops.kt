package com.mmcdole.kivo.android.ui

import android.view.KeyEvent
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue

// The logic of a season's row of two-stop episode cards (EpisodeCard.kt).

/**
 * Which stop of its cards a row is on, the one piece of state a row keeps for
 * them: read only as the focused card is drawn and as keys arrive, so moving
 * between stops recomposes nothing.
 */
@Stable
class EpisodeStops {
    var onText by mutableStateOf(false)
}

/** The stop an arrow moves to, from the text or not; null leaves the arrow to move focus. */
internal fun stopAfterArrow(keyCode: Int, onText: Boolean): Boolean? = when {
    keyCode == KeyEvent.KEYCODE_DPAD_DOWN && !onText -> true
    keyCode == KeyEvent.KEYCODE_DPAD_UP && onText -> false
    else -> null
}

/** Whether OK opens an episode's page rather than playing it: on its text, or an episode that can't play. */
internal fun okOpensPage(onText: Boolean, available: Boolean): Boolean = onText || !available

/** The remote's Play keys, which play the focused episode from either stop. */
internal val PlayKeys = setOf(KeyEvent.KEYCODE_MEDIA_PLAY, KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE)

/** How many of a row's cards fit across: a card opened on sits at the end of them, as few cards along as it takes to show it. */
internal const val EpisodeCardsAcross = 4

/** The first card a row shows as it opens, so that [entry] is in view; without it, the row's start. */
internal fun openingIndex(ids: List<String>, entry: String?): Int =
    (ids.indexOf(entry) - (EpisodeCardsAcross - 1)).coerceAtLeast(0)

/**
 * The item focus enters a row on, among those [shown]: the one last focused,
 * else [entry], else the first shown, as only what's shown can take focus.
 */
internal fun rowEntry(shown: List<String>, last: String?, entry: String?): String? =
    last?.takeIf { it in shown } ?: entry?.takeIf { it in shown } ?: shown.firstOrNull()

/** A synopsis on one line, without markup, or null when nothing's left of it. */
internal fun synopsisLine(text: String?): String? =
    text?.replace(Markup, " ")?.let(::oneLine)?.ifEmpty { null }

/** Paragraphs run together, for the most text in an overview's lines. */
internal fun oneLine(text: String): String = text.replace(Spaces, " ").trim()

private val Markup = Regex("<[^>]+>")
private val Spaces = Regex("\\s+")
