package com.mmcdole.kivo.android.ui

import android.graphics.Bitmap
import android.util.LruCache
import android.view.KeyEvent as AndroidKeyEvent
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.LinearOutSlowInEasing
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.State
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.painter.BitmapPainter
import androidx.compose.ui.graphics.painter.Painter
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.onClick
import androidx.compose.ui.semantics.onLongClick
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.tv.material3.Card
import androidx.tv.material3.CardDefaults
import androidx.tv.material3.Text
import coil3.compose.AsyncImage
import coil3.request.ImageRequest
import coil3.request.allowHardware
import coil3.request.crossfade
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.api.models.CollectionSummary
import com.mmcdole.kivo.api.models.ImageRef
import com.mmcdole.kivo.api.models.MovieInfo
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.display.timeLeft
import kotlinx.coroutines.launch

/** The signed-in server, for artwork and media URLs. */
val LocalSession = staticCompositionLocalOf<ServerSession> { error("no server session") }

/** Whether artwork may load: until it may, it shows its blurhash, as for cards a list is scrolling past. */
val LocalArtworkShown = staticCompositionLocalOf<State<Boolean>> { mutableStateOf(true) }

/** A card's shape: poster for movies, series and collections, and episodes in Favorites; landscape for episodes and videos; square for books. */
enum class CardShape(val aspect: Float) {
    Poster(KivoTokens.Aspect.poster),
    Landscape(KivoTokens.Aspect.landscape),
    Square(KivoTokens.Aspect.square),
}

/**
 * Artwork at the size it's shown, its blurhash first; without artwork, a
 * titled tile in the placeholder colour, never a broken image.
 */
@Composable
fun Artwork(
    image: ImageRef?,
    title: String,
    aspect: Float,
    width: Dp,
    modifier: Modifier = Modifier,
    corners: Dp = KivoTokens.Radius.md,
) {
    Box(
        modifier
            .aspectRatio(aspect)
            .clip(RoundedCornerShape(corners))
            .background(Kivo.colors.placeholder),
        contentAlignment = Alignment.Center,
    ) {
        ArtworkImage(image, title, width)
    }
}

/** The widest artwork decoded to an ordinary bitmap: cards and stills, not backdrops. */
private val CardArtworkMaxWidth = 400.dp

/** What [Artwork] shows in its frame: the image over its blurhash, or without one the title. */
@Composable
private fun ArtworkImage(image: ImageRef?, title: String, width: Dp) {
    if (image == null) {
        Text(
            title,
            style = TvType.smallStrong,
            color = Kivo.colors.onPlaceholder,
            textAlign = TextAlign.Center,
            maxLines = 3,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.padding(Kivo.space(3)),
        )
        return
    }
    val session = LocalSession.current
    val pixels = with(LocalDensity.current) { width.roundToPx() }
    val placeholder = remember(image.blurhash) { image.blurhash?.let(BlurhashCache::painter) }
    val context = LocalContext.current
    // Over its blurhash as it arrives; at once when it's already in memory.
    val request = remember(image.id, pixels) {
        ImageRequest.Builder(context)
            .data(session.session.imageUrl(image.id, pixels))
            // Cached by the image, not its URL, which moves with the server's address.
            .memoryCacheKey("${image.id}/$pixels")
            .diskCacheKey("${image.id}/$pixels")
            .crossfade(KivoTokens.Duration.fadeMs)
            // A screenful of cards draws faster from ordinary bitmaps; a backdrop
            // is one large image, kept in GPU memory rather than the app's heap.
            .allowHardware(width > CardArtworkMaxWidth)
            .build()
    }
    // Once it may load it keeps loading, whatever the list does next.
    val load = remember(request) { mutableStateOf(false) }
    if (LocalArtworkShown.current.value) load.value = true
    AsyncImage(
        model = if (load.value) request else null,
        contentDescription = null,
        imageLoader = session.images,
        placeholder = placeholder,
        error = placeholder,
        contentScale = ContentScale.Crop,
        modifier = Modifier.fillMaxSize(),
    )
}

/** Blurhashes decoded once each, small, as the same artwork shows again and again while lists scroll. */
private object BlurhashCache {
    private const val SIZE = 32
    private val bitmaps = LruCache<String, ImageBitmap>(256)

    fun painter(hash: String): Painter? = bitmap(hash)?.let(::BitmapPainter)

    fun bitmap(hash: String): ImageBitmap? = bitmaps[hash] ?: run {
        val pixels = Blurhash.decode(hash, SIZE, SIZE) ?: return null
        Bitmap.createBitmap(pixels, SIZE, SIZE, Bitmap.Config.ARGB_8888).asImageBitmap().also { bitmaps.put(hash, it) }
    }
}

/** Decodes the blurhash of [work]'s card ahead of it, such as off the main thread, so the card shows it without decoding it. */
fun prepareCard(work: WorkSummary, shape: CardShape) {
    cardImage(work, shape)?.blurhash?.let(BlurhashCache::bitmap)
}

/** The card's picture: a still or backdrop on landscape cards, else the poster. */
fun cardImage(work: WorkSummary, shape: CardShape): ImageRef? =
    if (shape == CardShape.Landscape) work.images.still ?: work.images.backdrop else work.images.poster

/** An episode, or a season bundled in Recently Added, goes by its series. */
fun cardTitle(work: WorkSummary): String = when {
    work.kind == "episode" -> work.episode?.seriesTitle ?: work.title
    work.kind == "season" && work.series?.recentlyAddedCount != null -> work.season?.seriesTitle ?: work.title
    else -> labelledTitle(work.title, work.movie)
}

/** A movie's title with its edition, "Blade Runner · Final Cut", when its cut is named. */
fun labelledTitle(title: String, movie: MovieInfo?): String = movie?.edition?.let { "$title · $it" } ?: title

/**
 * The card for a work, the same everywhere it appears: artwork
 * with progress, a played check or unplayed count, an episode's code, at
 * most one status badge; the title and a line under it.
 * Focus adds nothing but the ring and scale. A long press of OK, or the
 * Menu key, opens its menu. A null [work], one still loading, is the bare
 * card: focusable, so a grid scrolls on into it, and keeping focus as its
 * work arrives.
 */
@Composable
fun WorkCard(
    work: WorkSummary?,
    shape: CardShape,
    width: Dp,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val menu = LocalWorkMenu.current
    val title = work?.let(::cardTitle).orEmpty()
    Column(Modifier.width(width)) {
        CardSurface(
            onClick = onClick,
            onLongClick = work?.let { { menu(it) } },
            modifier = (if (work == null) modifier.testTag("placeholder") else modifier.menuKey(work, menu).testTag("card:${work.title}"))
                .aspectRatio(shape.aspect),
        ) {
            if (work != null) {
                ArtworkImage(cardImage(work, shape), title, width)
                CardOverlays(work, shape)
            }
        }
        if (work != null) {
            val colors = Kivo.colors
            Spacer(Modifier.height(CaptionGap))
            Text(title, style = TvType.smallStrong, color = colors.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
            cardSubtitle(work)?.let {
                Text(it, style = TvType.small, color = colors.textMuted, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

/**
 * A collection's card, its poster, or its name in the poster's place when
 * it has none, with how many works it has under it. A null [collection],
 * one still loading, is the bare card, as [WorkCard]'s is.
 */
@Composable
fun CollectionCard(collection: CollectionSummary?, width: Dp, onClick: () -> Unit, modifier: Modifier = Modifier) {
    Column(Modifier.width(width)) {
        CardSurface(
            onClick = onClick,
            onLongClick = null,
            modifier = (if (collection == null) modifier.testTag("placeholder") else modifier.testTag("card:${collection.name}"))
                .aspectRatio(CardShape.Poster.aspect),
        ) {
            if (collection != null) ArtworkImage(collection.images.poster, collection.name, width)
        }
        if (collection != null) {
            val colors = Kivo.colors
            Spacer(Modifier.height(CaptionGap))
            Text(collection.name, style = TvType.smallStrong, color = colors.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(collectionCount(collection.workCount, collection.source), style = TvType.small, color = colors.textMuted, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
    }
}

/** How many works a collection has: a movie collection counts its movies; a videos library's folder, what it holds. */
@Composable
fun collectionCount(count: Int, source: String?): String =
    pluralStringResource(if (source == "provider") R.plurals.movies else R.plurals.items, count, count)

/**
 * The card at a row's end that opens the rest: the row's card shape and
 * size, with "See all" and [detail], such as how many there are, on its face.
 */
@Composable
fun SeeAllCard(detail: String?, shape: CardShape, width: Dp, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val colors = Kivo.colors
    CardSurface(onClick = onClick, onLongClick = null, modifier = modifier.width(width).aspectRatio(shape.aspect).testTag("seeAll")) {
        Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.padding(Kivo.space(2))) {
            Text(stringResource(R.string.see_all), style = TvType.smallStrong, color = colors.text, textAlign = TextAlign.Center)
            if (detail != null) {
                Text(detail, style = TvType.small, color = colors.textMuted, textAlign = TextAlign.Center, maxLines = 2, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

/**
 * A card's face, as the TV's own cards behave: focus grows it and rings
 * it, OK clicks it as it comes up, and holding OK long-clicks it instead.
 * Its content is clipped to the card's corners, over the placeholder
 * colour. Focus and the scale change only how it's drawn, so neither
 * recomposes it.
 */
@Composable
private fun CardSurface(
    onClick: () -> Unit,
    onLongClick: (() -> Unit)?,
    modifier: Modifier = Modifier,
    content: @Composable BoxScope.() -> Unit,
) {
    val colors = Kivo.colors
    val scope = rememberCoroutineScope()
    val scale = remember { Animatable(1f) }
    val state = remember { CardState() }
    fun scaleTo(target: Float, millis: Int) {
        scope.launch { scale.animateTo(target, tween(millis, easing = LinearOutSlowInEasing)) }
    }
    Box(
        modifier
            .onFocusChanged {
                if (it.isFocused == state.focused) return@onFocusChanged
                state.focused = it.isFocused
                if (it.isFocused) scaleTo(KivoTokens.Tv.cardFocusScale, 300) else scaleTo(1f, 500)
            }
            .onKeyEvent { event ->
                if (event.key.nativeKeyCode !in OkKeys) return@onKeyEvent false
                val key = event.nativeKeyEvent
                when {
                    key.action == AndroidKeyEvent.ACTION_DOWN && key.repeatCount == 0 -> {
                        // A long press's release goes to the menu it opened, so each press starts afresh.
                        state.longPressed = false
                        scaleTo(1f, 120)
                    }
                    key.action == AndroidKeyEvent.ACTION_DOWN && key.repeatCount == 1 && onLongClick != null -> {
                        state.longPressed = true
                        scaleTo(KivoTokens.Tv.cardFocusScale, 300)
                        onLongClick()
                    }
                    key.action == AndroidKeyEvent.ACTION_UP -> {
                        if (!state.longPressed) {
                            scaleTo(KivoTokens.Tv.cardFocusScale, 300)
                            onClick()
                        }
                        state.longPressed = false
                    }
                }
                true
            }
            .semantics(mergeDescendants = true) {
                role = Role.Button
                onClick { onClick(); true }
                if (onLongClick != null) onLongClick { onLongClick(); true }
            }
            .focusable()
            .graphicsLayer {
                scaleX = scale.value
                scaleY = scale.value
            }
            .drawWithContent {
                drawContent()
                if (state.focused) {
                    drawRoundRect(
                        colors.focus,
                        cornerRadius = CornerRadius(KivoTokens.Radius.md.toPx()),
                        style = Stroke(KivoTokens.Tv.focusRingWidth.toPx()),
                    )
                }
            }
            .clip(RoundedCornerShape(KivoTokens.Radius.md))
            .background(colors.placeholder),
        contentAlignment = Alignment.Center,
        content = content,
    )
}

/** Whether a card has focus, which draws its ring, and whether OK held on it long-clicked. */
private class CardState {
    var focused by mutableStateOf(false)
    var longPressed = false
}

@Composable
private fun BoxScope.CardOverlays(work: WorkSummary, shape: CardShape) {
    val colors = Kivo.colors
    val landscape = shape == CardShape.Landscape
    val unplayed = work.series?.unplayedCount ?: work.season?.unplayedCount
    if (unplayed != null && unplayed > 0) {
        Text(
            unplayed.toString(),
            style = TvType.smallStrong,
            color = colors.onAccent,
            textAlign = TextAlign.Center,
            modifier = Modifier
                .align(Alignment.TopEnd)
                .padding(Kivo.space(2))
                .widthIn(min = 24.dp)
                .clip(RoundedCornerShape(KivoTokens.Radius.full))
                .background(colors.accent)
                .padding(horizontal = Kivo.space(1)),
        )
    } else if (work.userState?.played == true) {
        Box(
            Modifier
                .align(Alignment.TopEnd)
                .padding(Kivo.space(2))
                .size(24.dp)
                .clip(RoundedCornerShape(KivoTokens.Radius.full))
                .background(colors.accent),
            contentAlignment = Alignment.Center,
        ) { KivoIconView(KivoIcon.Check, size = 16.dp, tint = colors.onAccent) }
    }
    val code = work.episode?.let(display::episodeCode)
    if (code != null) {
        // A poster has no dark frame to sit on, so there the code is a badge.
        Text(
            code,
            style = TvType.smallStrong,
            color = colors.onBadge,
            modifier = if (landscape) {
                Modifier.align(Alignment.BottomStart).padding(Kivo.space(3))
            } else {
                Modifier
                    .align(Alignment.BottomStart)
                    .padding(Kivo.space(2))
                    .clip(RoundedCornerShape(KivoTokens.Radius.sm))
                    .background(colors.badge)
                    .padding(horizontal = Kivo.space(1))
            },
        )
    }
    val status = when (work.matchState) {
        "identifying" -> R.string.badge_identifying
        "unmatched" -> R.string.badge_unmatched
        else -> null
    }
    if (status != null) {
        Text(
            stringResource(status).uppercase(),
            style = TvType.smallStrong,
            color = colors.onBadge,
            modifier = Modifier
                .align(if (landscape || code != null) Alignment.BottomEnd else Alignment.BottomStart)
                .padding(Kivo.space(2))
                .clip(RoundedCornerShape(KivoTokens.Radius.sm))
                .background(colors.badge)
                .padding(horizontal = Kivo.space(1)),
        )
    }
    progressOf(work)?.let { ProgressBar(it, Modifier.align(Alignment.BottomCenter)) }
}

/** The share of a work played, for its progress bar; null when it isn't in progress. */
fun progressOf(work: WorkSummary): Float? {
    val position = work.userState?.positionMs ?: return null
    val duration = work.durationMs ?: return null
    if (duration <= 0) return null
    return (position.toFloat() / duration).coerceIn(0f, 1f)
}

/** The line under a card's title (the web's WorkCard), or null for none. */
@Composable
fun cardSubtitle(work: WorkSummary): String? {
    val added = work.series?.recentlyAddedCount
    return when (work.kind) {
        "series" -> when {
            added != null -> pluralStringResource(R.plurals.episodes, added, added)
            else -> {
                val last = work.series?.lastAirYear
                if (work.year != null && last != null && last != work.year) "${work.year}–$last" else work.year?.toString()
            }
        }
        "season" -> when {
            added != null -> "${work.title} · " + pluralStringResource(R.plurals.episodes, added, added)
            else -> work.season?.episodeCount?.let { pluralStringResource(R.plurals.episodes, it, it) }
        }
        "episode" -> {
            val position = work.userState?.positionMs
            val duration = work.durationMs
            if (position != null && duration != null) {
                work.title + " · " + display.timeLeft((duration - position).toLong())
            } else {
                work.title
            }
        }
        "movie" -> {
            val position = work.userState?.positionMs
            val duration = work.durationMs
            // In progress, as in Continue Watching, the time left says where you are.
            if (position != null && duration != null) {
                display.timeLeft((duration - position).toLong())
            } else {
                work.year?.toString()
            }
        }
        else -> work.year?.toString()
    }
}

/** A titled row of cards, as Home and the detail pages show them. */
@Composable
fun RowTitle(text: String, modifier: Modifier = Modifier) {
    Text(text, style = TvType.large, color = Kivo.colors.text, modifier = modifier.fillMaxWidth())
}

/** A person's round photo or initial, name and [detail], such as their character or "Actor · 23 titles"; it opens their page. */
@Composable
fun PersonCard(name: String, image: ImageRef?, detail: String?, onClick: () -> Unit, modifier: Modifier = Modifier) {
    Column(Modifier.width(112.dp)) {
        Card(
            onClick = onClick,
            modifier = modifier,
            shape = CardDefaults.shape(RoundedCornerShape(KivoTokens.Radius.full)),
            border = CardDefaults.border(focusedBorder = focusRing(RoundedCornerShape(KivoTokens.Radius.full))),
            scale = CardDefaults.scale(focusedScale = KivoTokens.Tv.cardFocusScale),
        ) { Artwork(image, name.take(1), 1f, 112.dp, corners = KivoTokens.Radius.full) }
        Text(name, style = TvType.smallStrong, color = Kivo.colors.text, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = Kivo.space(2)))
        detail?.let { Text(it, style = TvType.small, color = Kivo.colors.textMuted, maxLines = 1, overflow = TextOverflow.Ellipsis) }
    }
}

/** A credit's role as people read it: Actor, Director, Author. */
@Composable
fun roleLabel(role: String): String = when (role) {
    "actor" -> stringResource(R.string.role_actor)
    "director" -> stringResource(R.string.role_director)
    "writer" -> stringResource(R.string.role_writer)
    "author" -> stringResource(R.string.role_author)
    "narrator" -> stringResource(R.string.role_narrator)
    else -> role.replaceFirstChar { it.uppercase() }
}

/** A progress bar along a card's bottom edge. */
@Composable
fun ProgressBar(value: Float, modifier: Modifier = Modifier) {
    Box(modifier.fillMaxWidth().height(4.dp).background(Kivo.colors.progressTrack)) {
        Box(Modifier.fillMaxWidth(value).fillMaxHeight().background(Kivo.colors.progress))
    }
}
