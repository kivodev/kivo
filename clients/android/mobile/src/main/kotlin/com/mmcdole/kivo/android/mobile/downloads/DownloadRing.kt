package com.mmcdole.kivo.android.mobile.downloads

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

/**
 * A download's progress as a ring filling from the top: dashed while the
 * server has it, the accent while it's prepared or comes ([active]) and grey
 * otherwise, turning while it's prepared with nothing to measure yet, with
 * [center] in its middle.
 */
@Composable
fun DownloadRing(
    fraction: Float?,
    modifier: Modifier = Modifier,
    active: Boolean = true,
    dashed: Boolean = false,
    center: ImageVector? = null,
    size: Dp = 28.dp,
) {
    val track = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.35f)
    val tint = if (active) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant
    val shown by animateFloatAsState(fraction ?: 0f, label = "ring")
    val turning = active && fraction == null && !dashed
    val angle = if (turning) {
        val spin = rememberInfiniteTransition(label = "spin")
        spin.animateFloat(0f, 360f, infiniteRepeatable(tween(1000, easing = LinearEasing), RepeatMode.Restart), label = "angle").value
    } else {
        0f
    }
    Box(modifier.size(size), contentAlignment = Alignment.Center) {
        Canvas(Modifier.size(size)) {
            val width = 2.5.dp.toPx()
            val inset = width / 2
            val arc = Size(this.size.width - width, this.size.height - width)
            val dash = if (dashed) PathEffect.dashPathEffect(floatArrayOf(3.dp.toPx(), 3.dp.toPx())) else null
            drawArc(track, 0f, 360f, false, Offset(inset, inset), arc, style = Stroke(width, pathEffect = dash))
            when {
                turning -> drawArc(tint, angle - 90f, 90f, false, Offset(inset, inset), arc, style = Stroke(width, cap = StrokeCap.Round))
                shown > 0f -> drawArc(tint, -90f, 360f * shown.coerceAtMost(1f), false, Offset(inset, inset), arc, style = Stroke(width, cap = StrokeCap.Round))
            }
        }
        if (center != null) Icon(center, null, Modifier.size(size * 0.4f), tint = MaterialTheme.colorScheme.onSurface)
    }
}

/** [progress]'s ring: dashed while it's in line, turning while the server prepares it with nothing to measure yet, grey while it waits or is paused. */
@Composable
fun ProgressRing(progress: DownloadProgress, modifier: Modifier = Modifier, center: ImageVector? = null, size: Dp = 28.dp) {
    DownloadRing(
        progress.fraction,
        modifier,
        active = progress.active,
        dashed = progress.stage == Stage.Queued,
        center = center,
        size = size,
    )
}
