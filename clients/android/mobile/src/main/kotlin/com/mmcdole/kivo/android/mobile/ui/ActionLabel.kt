package com.mmcdole.kivo.android.mobile.ui

import androidx.compose.foundation.text.TextAutoSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color

/** A round action's word under its icon, on one line, smaller where large text leaves it too little room. */
@Composable
fun ActionLabel(label: String, color: Color, modifier: Modifier = Modifier) {
    val style = MaterialTheme.typography.labelMedium
    Text(
        label,
        modifier,
        color = color,
        maxLines = 1,
        style = style,
        autoSize = TextAutoSize.StepBased(minFontSize = style.fontSize * 0.6f, maxFontSize = style.fontSize),
    )
}
