package com.mmcdole.kivo.android.ui

import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithCache
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Canvas
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.drawscope.CanvasDrawScope
import androidx.compose.ui.unit.IntSize
import kotlin.math.roundToInt

/** How much smaller than the area an overlay is drawn before it's stretched over it. */
private const val OVERLAY_SCALE = 8

/**
 * Draws [layers], such as a backdrop's dimming and the gradients that fade
 * it into the page, over the content as one image, drawn small once for
 * the size and stretched over it: one pass a frame for all of them, where
 * a gradient drawn at full size costs a slow GPU a pass of its own.
 */
fun Modifier.overlay(vararg layers: Brush): Modifier = drawWithCache {
    val width = (size.width / OVERLAY_SCALE).roundToInt().coerceAtLeast(1)
    val height = (size.height / OVERLAY_SCALE).roundToInt().coerceAtLeast(1)
    val image = ImageBitmap(width, height)
    CanvasDrawScope().draw(this, layoutDirection, Canvas(image), Size(width.toFloat(), height.toFloat())) {
        for (layer in layers) drawRect(layer)
    }
    val stretched = IntSize(size.width.roundToInt(), size.height.roundToInt())
    onDrawWithContent {
        drawContent()
        drawImage(image, dstSize = stretched)
    }
}
