package com.mmcdole.kivo.android.mobile.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Dp
import coil3.compose.AsyncImage
import coil3.request.ImageRequest
import coil3.size.Dimension
import coil3.size.Size
import com.mmcdole.kivo.android.Blurhash
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.mobile.theme.LocalKivo
import com.mmcdole.kivo.api.models.ImageRef

/** The signed-in server, for the screens' artwork and view models. */
val LocalSession = staticCompositionLocalOf<ServerSession> { error("no server session") }

/** The widths artwork is asked for at, so a card asks for one the image cache already holds. */
private val Widths = intArrayOf(160, 240, 320, 480, 640, 960, 1280, 1920)

/** The first of [Widths] at least [px] wide. */
internal fun artworkWidth(px: Int): Int = Widths.firstOrNull { it >= px } ?: Widths.last()

/**
 * [image] at [width], through the session's image loader, cropped to fill
 * the box; its average colour while it loads, or the placeholder colour
 * when there's none.
 * It asks the server for the stored width that covers the box, and decodes
 * it at that size.
 */
@Composable
fun Artwork(image: ImageRef?, width: Dp, modifier: Modifier = Modifier, contentScale: ContentScale = ContentScale.Crop) {
    val placeholder = LocalKivo.current.placeholder
    if (image == null) {
        Box(modifier.background(placeholder))
        return
    }
    val session = LocalSession.current
    val context = LocalContext.current
    val px = with(LocalDensity.current) { artworkWidth(width.roundToPx()) }
    val loading = remember(image.blurhash) { image.blurhash?.let(Blurhash::averageColor)?.let { Color(it) } } ?: placeholder
    val request = remember(image.id, px) {
        ImageRequest.Builder(context).data(session.session.imageUrl(image.id, px)).size(Size(px, Dimension.Undefined)).build()
    }
    AsyncImage(
        model = request,
        contentDescription = null,
        imageLoader = session.images,
        modifier = modifier.background(loading),
        contentScale = contentScale,
    )
}
