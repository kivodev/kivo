package com.mmcdole.kivo.android.detail

import androidx.compose.foundation.focusGroup
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.Stable
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import com.mmcdole.kivo.android.ui.FocusMemory
import com.mmcdole.kivo.android.ui.focusKey

/** The button Up returns to: the one [last] focused, if it's still [offered], else Play, else Favorite, which every page has. */
internal fun returningButton(last: String, offered: Set<String>): String = when {
    last in offered -> last
    "play" in offered -> "play"
    else -> "favorite"
}

/**
 * How focus moves between a detail page's levels, read only as keys arrive:
 * the row of buttons, which focus coming into from anywhere returns to the
 * button it left, or Play until there has been one; the description, when it
 * doesn't fit; then whatever is [below] it. With [levels] off, Down from the
 * buttons is left to where things are laid out, as under a movie's buttons,
 * where the pickers come first.
 */
@Stable
internal class HeroLevels(val memory: FocusMemory, private val last: MutableState<String>) {
    val buttons = FocusRequester()
    val overview: FocusRequester = memory.requester(OVERVIEW)
    private val offered = HashSet<String>()

    var levels = true
    var overviewStops = false
    var below: FocusRequester? = null

    fun down(): FocusRequester = when {
        !levels -> FocusRequester.Default
        overviewStops -> overview
        else -> below ?: FocusRequester.Default
    }

    fun enterButtons() {
        memory.requester(returningButton(last.value, offered)).requestFocus()
    }

    fun offer(key: String) {
        offered += key
    }

    fun withdraw(key: String) {
        offered -= key
    }

    fun focused(key: String) {
        last.value = key
    }

    companion object {
        const val OVERVIEW = "overview"
    }
}

@Composable
internal fun rememberHeroLevels(memory: FocusMemory): HeroLevels {
    val last = rememberSaveable { mutableStateOf("play") }
    return remember(memory) { HeroLevels(memory, last) }
}

/** The row of a page's buttons, which focus coming in returns to the button it left. */
internal fun Modifier.heroButtons(hero: HeroLevels): Modifier =
    focusRequester(hero.buttons).focusProperties { onEnter = { hero.enterButtons() } }.focusGroup()

/** Says the row of [hero]'s buttons has the button [key] while this is composed, so focus can return to it. */
@Composable
internal fun OfferButton(hero: HeroLevels, key: String) {
    DisposableEffect(hero, key) {
        hero.offer(key)
        onDispose { hero.withdraw(key) }
    }
}

/** Makes this the button [key] of the row of [hero]'s buttons, which [OfferButton] offers. */
internal fun Modifier.heroButton(hero: HeroLevels, key: String): Modifier =
    focusProperties { down = hero.down() }
        .focusKey(hero.memory, key)
        .onFocusChanged { if (it.isFocused) hero.focused(key) }
