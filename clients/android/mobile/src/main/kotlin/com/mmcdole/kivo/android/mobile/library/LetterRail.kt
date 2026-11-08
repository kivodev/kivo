package com.mmcdole.kivo.android.mobile.library

import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.gestures.detectVerticalDragGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.mmcdole.kivo.core.data.GridPages.Companion.RAIL_LETTERS

/** The letter rail's width. */
val LetterRailWidth = 28.dp

/** The rail's letters of those a grid starts somewhere, in rail order. */
fun railLetters(starts: Collection<String>): List<String> = RAIL_LETTERS.filter { it in starts }

/**
 * The letter rail at a grid's right: the letters the grid has, [current]
 * in an accent circle. A tap or a drag along it jumps to the letter under
 * the finger, with a tick each time the letter changes.
 */
@Composable
fun LetterRail(letters: List<String>, current: String?, onLetter: (String) -> Unit, modifier: Modifier = Modifier) {
    val haptics = LocalHapticFeedback.current
    val jump by rememberUpdatedState(onLetter)
    var last by remember { mutableIntStateOf(-1) }
    var height by remember { mutableIntStateOf(1) }
    fun at(y: Float) {
        val index = (y / height * letters.size).toInt().coerceIn(0, letters.lastIndex)
        if (index != last) {
            last = index
            haptics.performHapticFeedback(HapticFeedbackType.SegmentFrequentTick)
            jump(letters[index])
        }
    }
    Column(
        modifier
            .width(LetterRailWidth)
            .fillMaxHeight()
            .testTag("letters")
            .semantics { contentDescription = current.orEmpty() }
            .onSizeChanged { height = it.height.coerceAtLeast(1) }
            .pointerInput(letters) {
                detectTapGestures(onPress = { offset ->
                    last = -1
                    at(offset.y)
                })
            }
            .pointerInput(letters) {
                detectVerticalDragGestures(
                    onDragStart = { at(it.y) },
                    onDragEnd = { last = -1 },
                ) { change, _ -> at(change.position.y) }
            },
        verticalArrangement = Arrangement.SpaceEvenly,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        for (letter in letters) {
            val active = letter == current
            Box(
                Modifier
                    .size(20.dp)
                    .then(if (active) Modifier.background(MaterialTheme.colorScheme.primary, CircleShape) else Modifier),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    letter,
                    style = MaterialTheme.typography.labelSmall,
                    color = if (active) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}
