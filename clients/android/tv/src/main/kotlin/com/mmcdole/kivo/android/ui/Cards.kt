package com.mmcdole.kivo.android.ui

import android.graphics.Bitmap
import android.util.LruCache
import android.view.KeyEvent as AndroidKeyEvent
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.AnimationVector1D
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
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.Stable
import androidx.compose.runtime.State
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.snapshotFlow
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.dropShadow
import androidx.compose.ui.draw.innerShadow
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.CompositingStrategy
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.shadow.Shadow
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
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.onClick
import androidx.compose.ui.semantics.onLongClick
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.tv.material3.Text
import coil3.compose.AsyncImage
import coil3.request.ImageRequest
import coil3.request.allowHardware
import coil3.request.crossfade
import com.mmcdole.kivo.android.tv.R
import com.mmcdole.kivo.android.ServerSession
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.api.models.CollectionSource
import com.mmcdole.kivo.api.models.CollectionSummary
import com.mmcdole.kivo.api.models.ImageRef
import com.mmcdole.kivo.api.models.MatchState
import com.mmcdole.kivo.api.models.MovieInfo
import com.mmcdole.kivo.api.models.Trailer
import com.mmcdole.kivo.api.models.WorkKind
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.data.CreditRole
import com.mmcdole.kivo.core.display.episodeCode
import com.mmcdole.kivo.core.display.timeLeft
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.filterNotNull
import kotlinx.coroutines.launch

/** The signed-in server, for artwork and media URLs. */
val LocalSession = staticCompositionLocalOf<ServerSession> { error("no server session") }

/** Whether artwork may load: until it may, it shows its blurhash, as for cards a list is scrolling past. */
val LocalArtworkShown = staticCompositionLocalOf<State<Boolean>> { mutableStateOf(true) }

/**
 * How much of a focused card's light shows, from 0 to 1. A screen whose
 * focus a held key moves turns it off until focus rests, then fades it
 * back: a Chromecast with Google TV can't draw a card's blurs at a held
 * key's pace, and a layer with no alpha isn't drawn at all.
 */
val LocalCardLight = staticCompositionLocalOf<State<Float>> { mutableFloatStateOf(1f) }

/**
 * Whether [WorkCard]s show their captions and played badges yet, for a list whose rows or lines are built as it
 * scrolls: measuring a row's text costs as much as the rest of building it, so rows passing under a held key skip
 * it, with the captions' room kept. Null shows them at once, in only the room they need.
 */
val LocalCaptionsShown = staticCompositionLocalOf<State<Boolean>?> { null }

/** How long focus stays on a card before it counts as resting. */
private const val FOCUS_REST_MS = 300L

/**
 * A [LocalCardLight] for a list of cards, whose [focused] card it follows: it goes out at once as focus moves on
 * before resting, as with a held key, and fades in as focus rests. Read only to place the light, so it recomposes
 * nothing as it moves.
 */
@Composable
fun rememberCardLight(focused: () -> Any?): State<Float> {
    val follow = rememberUpdatedState(focused)
    val browsing = remember { mutableStateOf(false) }
    LaunchedEffect(Unit) {
        var resting = true
        // The first card to take focus isn't a move.
        snapshotFlow { follow.value() }.filterNotNull().drop(1).collectLatest {
            if (!resting) browsing.value = true
            resting = false
            delay(FOCUS_REST_MS)
            browsing.value = false
            resting = true
        }
    }
    val light = remember { Animatable(1f) }
    LaunchedEffect(Unit) {
        snapshotFlow { browsing.value }.collectLatest { if (it) light.snapTo(0f) else light.animateTo(1f, fadeSpec()) }
    }
    return remember { light.asState() }
}

/** A card's shape: poster for movies, series and collections, and episodes in lists; landscape for episodes and videos; square for books. */
enum class CardShape(val aspect: Float) {
    Poster(KivoTokens.Aspect.poster),
    Landscape(KivoTokens.Aspect.landscape),
    Square(KivoTokens.Aspect.square),
}

/** The shape a list's card takes for its own work: a video landscape, an episode by its series' poster, a book square. */
fun cardShapeOf(work: WorkSummary): CardShape = when (work.kind) {
    WorkKind.Video, WorkKind.Extra -> CardShape.Landscape
    WorkKind.Book -> CardShape.Square
    else -> CardShape.Poster
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
internal fun ArtworkImage(image: ImageRef?, title: String, width: Dp) {
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
internal object BlurhashCache {
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
    work.kind == WorkKind.Episode -> work.episode?.seriesTitle ?: work.title
    work.kind == WorkKind.Season && work.series?.recentlyAddedCount != null -> work.season?.seriesTitle ?: work.title
    else -> labelledTitle(work.title, work.movie)
}

/** A movie's title with its edition, "Blade Runner · Final Cut", when its cut is named. */
fun labelledTitle(title: String, movie: MovieInfo?): String = movie?.edition?.let { "$title · $it" } ?: title

/**
 * The card for a work, the same everywhere it appears: artwork
 * with progress, a played check or unplayed count, an episode's code, at
 * most one status badge; the title and a line under it.
 * Focus adds nothing but the light and scale. A long press of OK, or the
 * Menu key, opens its menu. [mark] is a short mark at the artwork's bottom
 * end, such as a trailer's running time. A null [work], one still loading, is the bare
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
    mark: String? = null,
) {
    val menu = LocalWorkMenu.current
    val title = work?.let(::cardTitle).orEmpty()
    val focus = rememberCardFocus()
    Column(Modifier.width(width).raisedOnFocus()) {
        CardSurface(
            onClick = onClick,
            onLongClick = work?.let { { menu(it) } },
            modifier = (if (work == null) modifier.testTag("placeholder") else modifier.menuKey(work, menu).testTag("card:${work.title}"))
                .aspectRatio(shape.aspect),
            focus = focus,
        ) {
            if (work != null) {
                ArtworkImage(cardImage(work, shape), title, width)
                CardOverlays(work, shape)
                if (mark != null) CardMark(mark, Alignment.BottomEnd)
            }
        }
        if (work != null) WorkCaption(work, title, focus)
    }
}

/** A short mark on a card's artwork. */
@Composable
private fun BoxScope.CardMark(text: String, alignment: Alignment) {
    val colors = Kivo.colors
    Text(
        text,
        style = TvType.smallStrong,
        color = colors.onBadge,
        modifier = Modifier
            .align(alignment)
            .padding(Kivo.space(2))
            .clip(RoundedCornerShape(KivoTokens.Radius.sm))
            .background(colors.badge)
            .padding(horizontal = Kivo.space(1)),
    )
}

/**
 * A trailer on YouTube: its thumbnail, marked as leaving for YouTube before
 * it's opened, then its title. With no thumbnail the tile is bare, as the
 * title is under it. [onClick] opens it there.
 */
@Composable
fun TrailerCard(trailer: Trailer, width: Dp, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val focus = rememberCardFocus()
    val opens = stringResource(R.string.opens_in_youtube)
    Column(Modifier.width(width).raisedOnFocus()) {
        CardSurface(
            onClick = onClick,
            onLongClick = null,
            modifier = modifier
                .testTag("trailer:${trailer.title}")
                .semantics { contentDescription = "${trailer.title}, $opens" }
                .aspectRatio(CardShape.Landscape.aspect),
            focus = focus,
        ) {
            ArtworkImage(trailer.image, "", width)
            CardMark("YouTube ↗", Alignment.BottomStart)
        }
        val colors = Kivo.colors
        Column(Modifier.cardCaption(focus)) {
            Spacer(Modifier.height(CaptionGap))
            Text(trailer.title, style = TvType.smallStrong, color = colors.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
    }
}

/** Apart from [WorkCard], so its captions arriving recomposes only them. */
@Composable
private fun WorkCaption(work: WorkSummary, title: String, focus: CardFocus) {
    val shown = LocalCaptionsShown.current
    val room = shown?.let {
        with(LocalDensity.current) { CaptionGap + TvType.smallStrong.lineHeight.toDp() + TvType.small.lineHeight.toDp() }
    }
    if (shown?.value == false) {
        Spacer(Modifier.height(room!!))
        return
    }
    val colors = Kivo.colors
    Column(Modifier.cardCaption(focus).then(if (room != null) Modifier.height(room) else Modifier)) {
        Spacer(Modifier.height(CaptionGap))
        Text(title, style = TvType.smallStrong, color = colors.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
        cardSubtitle(work)?.let {
            Text(it, style = TvType.small, color = colors.textMuted, maxLines = 1, overflow = TextOverflow.Ellipsis)
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
    val focus = rememberCardFocus()
    Column(Modifier.width(width).raisedOnFocus()) {
        CardSurface(
            onClick = onClick,
            onLongClick = null,
            modifier = (if (collection == null) modifier.testTag("placeholder") else modifier.testTag("card:${collection.name}"))
                .aspectRatio(CardShape.Poster.aspect),
            focus = focus,
        ) {
            if (collection != null) ArtworkImage(collection.images.poster, collection.name, width)
        }
        if (collection != null) {
            val colors = Kivo.colors
            Column(Modifier.cardCaption(focus)) {
                Spacer(Modifier.height(CaptionGap))
                Text(collection.name, style = TvType.smallStrong, color = colors.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(collectionCount(collection.workCount, collection.source), style = TvType.small, color = colors.textMuted, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

/** How many works a collection has: a movie collection counts its movies; a videos library's folder, what it holds. */
@Composable
fun collectionCount(count: Int, source: CollectionSource?): String =
    pluralStringResource(if (source == CollectionSource.Provider) R.plurals.movies else R.plurals.items, count, count)

/**
 * The card at a row's end that opens the rest: the row's card shape and
 * size, with "See all" and [detail], such as how many there are, on its face.
 */
@Composable
fun SeeAllCard(detail: String?, shape: CardShape, width: Dp, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val colors = Kivo.colors
    CardSurface(onClick = onClick, onLongClick = null, modifier = modifier.raisedOnFocus().width(width).aspectRatio(shape.aspect).testTag("seeAll")) {
        Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.padding(Kivo.space(2))) {
            Text(stringResource(R.string.see_all), style = TvType.smallStrong, color = colors.text, textAlign = TextAlign.Center)
            if (detail != null) {
                Text(detail, style = TvType.small, color = colors.textMuted, textAlign = TextAlign.Center, maxLines = 2, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

/** A card's corners. */
private val CardCorners = RoundedCornerShape(KivoTokens.Radius.md)

/**
 * A card's focus, which its face and its caption share: how far its light
 * has come on, from 0 to 1, which the glow, the sheen and the caption's
 * shift follow, and its scale, which an OK press dips. Both ease as the
 * web TV's do and are read only as the card is drawn, so focus moving
 * recomposes nothing.
 */
@Stable
class CardFocus internal constructor(private val scope: CoroutineScope, private val rests: () -> Boolean = { false }) {
    internal val light = Animatable(0f)
    internal val scale = Animatable(1f)
    private var focused = false

    /** Whether OK held on the card long-clicked, so its release doesn't click as well. */
    internal var longPressed = false

    internal fun focus(on: Boolean) {
        if (on == focused) return
        focused = on
        settle()
    }

    /** Eases to how the card looks now: lit and grown with focus, unless it [rests] with focus on a stop of its own. */
    internal fun settle() {
        val lit = focused && !rests()
        ease(light, if (lit) 1f else 0f)
        scaleTo(if (lit) KivoTokens.Tv.cardFocusScale else 1f)
    }

    /** Back up from an OK press's dip, to the size it has with focus. */
    internal fun scaleUp() = scaleTo(if (rests()) 1f else KivoTokens.Tv.cardFocusScale)

    internal fun scaleTo(target: Float, millis: Int = KivoTokens.Duration.fadeMs) = ease(scale, target, millis)

    private fun ease(value: Animatable<Float, AnimationVector1D>, target: Float, millis: Int = KivoTokens.Duration.fadeMs) {
        scope.launch { value.animateTo(target, tween(millis, easing = KivoTokens.Ease.out)) }
    }
}

@Composable
fun rememberCardFocus(): CardFocus {
    val scope = rememberCoroutineScope()
    return remember { CardFocus(scope) }
}

/** A card's focus for a card with a second stop: while [rests], focus leaves its face as it is without focus. */
@Composable
internal fun rememberCardFocus(rests: () -> Boolean): CardFocus {
    val scope = rememberCoroutineScope()
    return remember { CardFocus(scope, rests) }
}

/** A card's caption, which moves down clear of the focused card's grown artwork, at its own size so it stays sharp. */
fun Modifier.cardCaption(focus: CardFocus): Modifier = graphicsLayer {
    translationY = KivoTokens.Tv.cardCaptionShift.toPx() * focus.light.value
}

/**
 * A card's face, as the web TV's cards behave: focus grows it, lit by a
 * glow around it over a deep shadow and a sheen across its top; OK clicks
 * it as it comes up, and holding OK long-clicks it instead. Its content is
 * clipped to [shape], over the placeholder colour. Give the card's whole
 * column [raisedOnFocus], so the glow lies over its neighbours, and its
 * caption [cardCaption] with the same [focus].
 */
@Composable
internal fun CardSurface(
    onClick: () -> Unit,
    onLongClick: (() -> Unit)?,
    modifier: Modifier = Modifier,
    shape: Shape = CardCorners,
    focus: CardFocus = rememberCardFocus(),
    content: @Composable BoxScope.() -> Unit,
) {
    val colors = Kivo.colors
    Box(
        modifier
            .onFocusChanged { focus.focus(it.isFocused) }
            .onKeyEvent { event ->
                if (event.key.nativeKeyCode !in OkKeys) return@onKeyEvent false
                val key = event.nativeKeyEvent
                when {
                    key.action == AndroidKeyEvent.ACTION_DOWN && key.repeatCount == 0 -> {
                        // A long press's release goes to the menu it opened, so each press starts afresh.
                        focus.longPressed = false
                        focus.scaleTo(1f, PressMs)
                    }
                    key.action == AndroidKeyEvent.ACTION_DOWN && key.repeatCount == 1 && onLongClick != null -> {
                        focus.longPressed = true
                        focus.scaleUp()
                        onLongClick()
                    }
                    key.action == AndroidKeyEvent.ACTION_UP -> {
                        if (!focus.longPressed) {
                            focus.scaleUp()
                            onClick()
                        }
                        focus.longPressed = false
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
                scaleX = focus.scale.value
                scaleY = focus.scale.value
            },
    ) {
        // The light is drawn once, each blur rendered to a bitmap Compose keeps for the card's size and shape, and
        // fades on its layer's alpha alone, so it costs the GPU a few bitmap draws and the main thread nothing.
        val light = LocalCardLight.current
        Box(Modifier.matchParentSize().cardLight(focus, light).dropShadows(shape, CardGlow))
        Box(Modifier.fillMaxSize().clip(shape).background(colors.placeholder), contentAlignment = Alignment.Center, content = content)
        Box(Modifier.matchParentSize().cardLight(focus, light).innerShadows(shape, KivoTokens.Tv.CardFocus.sheen))
    }
}

/** The web TV's glow and deep shadow behind a focused card, the glow's alpha 0.38 to its 0.32. */
private val CardGlow = KivoTokens.Tv.CardFocus.glow.let { (glow, shadow) ->
    listOf(Shadow(radius = glow.radius, color = glow.color.copy(alpha = 0.38f), spread = glow.spread, offset = glow.offset), shadow)
}

/** How long an OK press takes to dip a card. */
private const val PressMs = 120

/**
 * A layer of a card's light, as on as the card's focus and as the screen's [light] allows. Its alpha falls on each
 * shadow drawn, not on a buffer of them all.
 */
private fun Modifier.cardLight(focus: CardFocus, light: State<Float>): Modifier = graphicsLayer {
    alpha = focus.light.value * light.value
    compositingStrategy = CompositingStrategy.ModulateAlpha
}

/** [shadows] behind this, the first on top. */
private fun Modifier.dropShadows(shape: Shape, shadows: List<Shadow>): Modifier =
    shadows.asReversed().fold(this) { modifier, shadow -> modifier.dropShadow(shape, shadow) }

/** [shadows] inside this, the first on top. */
private fun Modifier.innerShadows(shape: Shape, shadows: List<Shadow>): Modifier =
    shadows.asReversed().fold(this) { modifier, shadow -> modifier.innerShadow(shape, shadow) }

@Composable
internal fun BoxScope.CardOverlays(work: WorkSummary, shape: CardShape, showCode: Boolean = true) {
    val colors = Kivo.colors
    val landscape = shape == CardShape.Landscape
    PlayedBadge(work)
    val code = work.episode?.takeIf { showCode }?.let(display::episodeCode)
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
        MatchState.Identifying -> R.string.badge_identifying
        MatchState.Unmatched -> R.string.badge_unmatched
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

/**
 * A card's unplayed count or played check, shown with its captions ([LocalCaptionsShown]): a line of shows' counts
 * costs a held key more than the rest of the line. Apart from [CardOverlays], so its arriving recomposes only it.
 */
@Composable
private fun BoxScope.PlayedBadge(work: WorkSummary) {
    if (LocalCaptionsShown.current?.value == false) return
    val colors = Kivo.colors
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
        WorkKind.Series -> when {
            added != null -> pluralStringResource(R.plurals.episodes, added, added)
            else -> {
                val last = work.series?.lastAirYear
                if (work.year != null && last != null && last != work.year) "${work.year}–$last" else work.year?.toString()
            }
        }
        WorkKind.Season -> when {
            added != null -> "${work.title} · " + pluralStringResource(R.plurals.episodes, added, added)
            else -> work.season?.episodeCount?.let { pluralStringResource(R.plurals.episodes, it, it) }
        }
        WorkKind.Episode -> {
            val position = work.userState?.positionMs
            val duration = work.durationMs
            if (position != null && duration != null) {
                work.title + " · " + display.timeLeft((duration - position).toLong())
            } else {
                work.title
            }
        }
        WorkKind.Movie -> {
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

private val PersonCardWidth = 112.dp

/** A person's round photo or initial, name and [detail], such as their character or "Actor · 23 titles"; it opens their page. */
@Composable
fun PersonCard(name: String, image: ImageRef?, detail: String?, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val focus = rememberCardFocus()
    Column(Modifier.width(PersonCardWidth).raisedOnFocus()) {
        CardSurface(
            onClick = onClick,
            onLongClick = null,
            modifier = modifier.aspectRatio(1f),
            shape = RoundedCornerShape(KivoTokens.Radius.full),
            focus = focus,
        ) { Artwork(image, name.take(1), 1f, PersonCardWidth, corners = KivoTokens.Radius.full) }
        Column(Modifier.cardCaption(focus)) {
            Text(name, style = TvType.smallStrong, color = Kivo.colors.text, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = Kivo.space(2)))
            detail?.let { Text(it, style = TvType.small, color = Kivo.colors.textMuted, maxLines = 1, overflow = TextOverflow.Ellipsis) }
        }
    }
}

/** A credit's role as people read it: Actor, Director, Author. */
@Composable
fun roleLabel(role: String): String = when (CreditRole.of(role)) {
    CreditRole.Actor -> stringResource(R.string.role_actor)
    CreditRole.Director -> stringResource(R.string.role_director)
    CreditRole.Writer -> stringResource(R.string.role_writer)
    CreditRole.Author -> stringResource(R.string.role_author)
    CreditRole.Narrator -> stringResource(R.string.role_narrator)
    else -> role.replaceFirstChar { it.uppercase() }
}

/** A progress bar along a card's bottom edge. */
@Composable
fun ProgressBar(value: Float, modifier: Modifier = Modifier) {
    Box(modifier.fillMaxWidth().height(4.dp).background(Kivo.colors.progressTrack)) {
        Box(Modifier.fillMaxWidth(value).fillMaxHeight().background(Kivo.colors.progress))
    }
}
