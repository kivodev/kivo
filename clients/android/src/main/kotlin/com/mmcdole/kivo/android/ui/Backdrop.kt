package com.mmcdole.kivo.android.ui

import android.graphics.Bitmap
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.geometry.toRect
import androidx.compose.ui.graphics.Canvas
import androidx.compose.ui.graphics.Paint
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Dp
import coil3.compose.AsyncImage
import coil3.request.ImageRequest
import coil3.request.transformations
import coil3.size.Size
import coil3.transform.Transformation
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.api.models.ImageRef
import androidx.compose.ui.geometry.Size as DrawSize

/**
 * A screen's backdrop: [image], decoded for [width], with [layers], such as
 * its dimming and the gradients that fade it into the page, painted into it
 * once as it's decoded, so each frame draws it in one pass. Drawn as layers
 * over the image instead, a slow TV's GPU paints the backdrop's area again
 * for each of them every frame, which on a Chromecast with Google TV was
 * about half of each frame while Home's rows moved over it. Until it
 * arrives its blurhash shows, under the layers.
 */
@Composable
fun Backdrop(image: ImageRef, width: Dp, layers: List<Brush>, modifier: Modifier = Modifier) {
    val session = LocalSession.current
    val context = LocalContext.current
    val pixels = with(LocalDensity.current) { width.roundToPx() }
    val placeholder = remember(image.blurhash) { image.blurhash?.let(BlurhashCache::painter) }
    val painted = remember(layers) { PaintedOver(layers) }
    val request = remember(image.id, pixels, painted) {
        ImageRequest.Builder(context)
            .data(session.session.imageUrl(image.id, pixels))
            // Cached by the image, not its URL, which moves with the server's address; in memory as painted.
            .memoryCacheKey("${image.id}/$pixels/${painted.cacheKey}")
            .diskCacheKey("${image.id}/$pixels")
            .transformations(painted)
            .build()
    }
    var shown by remember(request) { mutableStateOf(false) }
    Box(
        modifier
            .aspectRatio(KivoTokens.Aspect.landscape)
            .then(if (shown) Modifier else Modifier.background(Kivo.colors.placeholder).overlay(*layers.toTypedArray())),
    ) {
        AsyncImage(
            model = request,
            contentDescription = null,
            imageLoader = session.images,
            placeholder = placeholder,
            error = placeholder,
            onSuccess = { shown = true },
            contentScale = ContentScale.Crop,
            modifier = Modifier.fillMaxSize(),
        )
    }
}

/**
 * Paints [layers] over the decoded image, each across all of it, dithered, as
 * a dark gradient over a screen's width has too few 8-bit steps not to band.
 * The dither is paid once, here, and never as it's drawn.
 */
private data class PaintedOver(private val layers: List<Brush>) : Transformation() {
    override val cacheKey = "paintedOver${layers.hashCode()}"

    override suspend fun transform(input: Bitmap, size: Size): Bitmap {
        val output = input.copy(Bitmap.Config.ARGB_8888, true)
        val area = DrawSize(output.width.toFloat(), output.height.toFloat())
        val canvas = Canvas(output.asImageBitmap())
        val paint = Paint().apply { asFrameworkPaint().isDither = true }
        for (layer in layers) {
            layer.applyTo(area, paint, 1f)
            canvas.drawRect(area.toRect(), paint)
        }
        return output
    }
}
