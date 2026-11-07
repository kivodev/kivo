package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.interaction.InteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.saveable.Saver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.focus.FocusEventModifierNode
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.FocusState
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.layout.Measurable
import androidx.compose.ui.layout.MeasureResult
import androidx.compose.ui.layout.MeasureScope
import androidx.compose.ui.node.LayoutModifierNode
import androidx.compose.ui.node.ModifierNodeElement
import androidx.compose.ui.node.invalidatePlacement
import androidx.compose.ui.platform.InspectorInfo
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalWindowInfo
import androidx.compose.ui.unit.Constraints
import androidx.tv.material3.Border
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.LitColors
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

// Focus on the TV is the web TV's, never colour alone: a card's artwork
// grows on a glow (Cards.kt), and a control, a field or a list's row turns
// white with dark text, lit. Only a control that keeps focus while disabled
// takes a ring, as tv-material has no colours for it.

/** A focused control's colours: the light theme's raised surface, white, and its text. */
object Lit {
    val container = KivoTokens.light.surfaceRaised
    val content = KivoTokens.light.text
}

/** [content] in the light theme's colours while the control of [interactions] has focus, so its own muted lines and accents read on the white. */
@Composable
fun LitOnFocus(interactions: InteractionSource, content: @Composable () -> Unit) {
    val focused by interactions.collectIsFocusedAsState()
    LitColors(focused, content)
}

/** The ring of a control that has focus while disabled, which has no lit colours to show it. */
@Composable
fun focusRing(shape: androidx.compose.ui.graphics.Shape = RoundedCornerShape(KivoTokens.Radius.md)) =
    Border(BorderStroke(KivoTokens.Tv.focusRingWidth, Kivo.colors.focus), shape = shape)

/**
 * Raises this above its siblings while it or something in it has focus, so
 * a focused card's glow lies over its neighbours, and for as long after as
 * the glow takes to fade, so a card losing focus never goes under the next
 * one's. It changes only where this is placed, so focus moving recomposes
 * nothing. Each layout between a card and its lazy list needs it.
 */
fun Modifier.raisedOnFocus(): Modifier = this then RaisedOnFocusElement

private data object RaisedOnFocusElement : ModifierNodeElement<RaisedOnFocusNode>() {
    override fun create() = RaisedOnFocusNode()

    override fun update(node: RaisedOnFocusNode) = Unit

    override fun InspectorInfo.inspectableProperties() {
        name = "raisedOnFocus"
    }
}

private class RaisedOnFocusNode : Modifier.Node(), FocusEventModifierNode, LayoutModifierNode {
    private var raised = false
    private var lowering: Job? = null

    override fun onFocusEvent(focusState: FocusState) {
        if (focusState.hasFocus) {
            lowering?.cancel()
            lowering = null
            raise(true)
        } else if (raised && lowering == null) {
            lowering = coroutineScope.launch {
                delay(KivoTokens.Duration.fadeMs.toLong())
                lowering = null
                raise(false)
            }
        }
    }

    private fun raise(to: Boolean) {
        if (raised == to) return
        raised = to
        invalidatePlacement()
    }

    override fun MeasureScope.measure(measurable: Measurable, constraints: Constraints): MeasureResult {
        val placeable = measurable.measure(constraints)
        return layout(placeable.width, placeable.height) { placeable.place(0, 0, zIndex = if (raised) 1f else 0f) }
    }
}

/** Requests focus once the target is laid out and takes it, trying each frame, for up to 60, as a lazy list places its items. */
suspend fun focusWhenPlaced(requester: FocusRequester) = focusWhenPlaced { requester }

/** As [focusWhenPlaced], asking each frame for the target, which may not be known yet. */
suspend fun focusWhenPlaced(requester: () -> FocusRequester?) {
    repeat(60) {
        val target = requester()
        if (target != null && runCatching { target.requestFocus(FocusDirection.Enter) }.getOrDefault(false)) return
        withFrameNanos {}
    }
}

/**
 * Which item of a screen had focus, kept with the screen's saved state, so
 * Back returns focus to the item you came from rather than recomputing it,
 * and a live update that keeps the item keeps the focus.
 */
@Stable
class FocusMemory(private var last: String?) {
    private val requesters = HashMap<String, FocusRequester>()

    fun requester(key: String): FocusRequester = requesters.getOrPut(key) { FocusRequester() }

    fun focused(key: String) {
        last = key
    }

    /** Focuses the remembered item, else [initial]: the screen's defined first focus. */
    suspend fun restore(initial: String?) {
        val key = last ?: initial ?: return
        focusWhenPlaced(requester(key))
    }

    /** The item [gone] left the screen, as a placeholder with nothing to become: focus on it goes to [by]. */
    suspend fun replaced(gone: String, by: String) {
        if (last != gone) return
        last = by
        focusWhenPlaced(requester(by))
    }

    companion object {
        val Saver = Saver<FocusMemory, String>(save = { it.last.orEmpty() }, restore = { FocusMemory(it.ifEmpty { null }) })
    }
}

@Composable
fun rememberFocusMemory(): FocusMemory = rememberSaveable(saver = FocusMemory.Saver) { FocusMemory(null) }

/** Makes this the item [key] of [memory]: focusable by it, and remembered when focused. */
fun Modifier.focusKey(memory: FocusMemory, key: String): Modifier =
    focusRequester(memory.requester(key)).onFocusChanged { if (it.isFocused) memory.focused(key) }

/** Moves focus to the rail's current destination: for a screen left with nothing to focus, such as a list emptied while open. */
val LocalFocusRail = staticCompositionLocalOf<() -> Unit> { {} }

/** Hands focus to the rail as it's shown: in a screen's state that has nothing to focus, such as an empty one. */
@Composable
fun FocusRail() {
    val focusRail = LocalFocusRail.current
    LaunchedEffect(Unit) { focusRail() }
}

/**
 * Focuses [requester] as a screen opens, and again as its keys change, once
 * the window has focus. A screen that replaces another whose control had
 * focus, such as sign-in after Change server, drops that focus first, so
 * the control it focuses shows its focus.
 */
@Composable
fun FocusOnOpen(requester: FocusRequester, vararg keys: Any?) {
    val windowFocused = LocalWindowInfo.current.isWindowFocused
    val focusManager = LocalFocusManager.current
    LaunchedEffect(*keys, windowFocused) {
        if (!windowFocused) return@LaunchedEffect
        focusManager.clearFocus(force = true)
        focusWhenPlaced(requester)
    }
}
