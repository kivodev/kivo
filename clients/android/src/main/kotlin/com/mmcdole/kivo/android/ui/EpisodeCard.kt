@file:OptIn(ExperimentalFoundationApi::class)

package com.mmcdole.kivo.android.ui

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.focusGroup
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyListPrefetchStrategy
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicText
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ColorProducer
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.layout.layout
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.tv.material3.Text
import com.mmcdole.kivo.android.R
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import com.mmcdole.kivo.api.models.WorkSummary
import com.mmcdole.kivo.core.display.runtime
import com.mmcdole.kivo.core.display.timeLeft
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withTimeoutOrNull

/** The focus key of an episode's card, by which Back returns to it. */
fun episodeCardKey(id: String) = "card/$id"

/**
 * A row's place on a detail page, which lays rows out tighter than their room
 * for focus: its [top] and [bottom] of that room overlap what's beside them.
 * This only places it, so it costs no layer.
 */
fun Modifier.rowPlace(top: Dp = 0.dp, bottom: Dp = 0.dp): Modifier = layout { measurable, constraints ->
    val placeable = measurable.measure(constraints)
    val up = top.roundToPx()
    layout(constraints.maxWidth, placeable.height - up - bottom.roundToPx()) { placeable.place(0, -up) }
}

/**
 * A season's episodes as a row of cards, as the Samsung TV app shows them.
 * Each card has two stops. Focus lands on the thumbnail, where OK plays the
 * episode; Down moves to its text, where OK opens its page. Left and Right
 * keep the stop along the row, Up from the text returns to the thumbnail, and
 * leaving the row resets it.
 *
 * It's one focus target a card, its thumbnail, and one [EpisodeStops] a row
 * that only drawing and keys read, so changing stop recomposes and measures
 * nothing: the focused card draws its play mark or its lit text, and its
 * thumbnail eases to rest as any card does losing focus.
 *
 * Focus enters on the card last focused, else [entry], which the row opens
 * showing. [currentId]'s card is ringed and tagged as the page's own. [up] is
 * where Up goes from a thumbnail. [group] takes focus into the row.
 */
@Composable
fun EpisodeCardRow(
    episodes: List<WorkSummary>,
    entry: String?,
    currentId: String?,
    memory: FocusMemory,
    group: FocusRequester,
    navigator: Navigator,
    modifier: Modifier = Modifier,
    up: () -> FocusRequester = { FocusRequester.Default },
) {
    val stops = remember { EpisodeStops() }
    val cards = rememberLazyListState(
        initialFirstVisibleItemIndex = remember { openingIndex(episodes.map { it.id }, entry) },
        prefetchStrategy = remember { LazyListPrefetchStrategy(EpisodeCardsAcross) },
    )
    var last by rememberSaveable { mutableStateOf<String?>(null) }
    // A row with no synopses keeps its cards a line shorter.
    val lines = remember(episodes) { if (episodes.any { synopsisLine(it.episode?.overview) != null }) 2 else 1 }
    // A card's artwork loads once the row has stopped, or it has stayed on screen a while, so cards passed aren't decoded.
    val artwork = remember { mutableStateOf(false) }
    LaunchedEffect(cards) {
        while (true) {
            withTimeoutOrNull(ArtworkDwellMs) { cards.awaitSettled() }
            artwork.value = true
            snapshotFlow { cards.isScrollInProgress }.first { it }
            artwork.value = false
        }
    }
    Box(
        modifier
            .onFocusChanged { if (!it.hasFocus) stops.onText = false }
            .focusRequester(group)
            .focusProperties {
                onEnter = {
                    val shown = cards.layoutInfo.visibleItemsInfo.mapNotNull { it.key as? String }
                    rowEntry(shown, last, entry)?.let { memory.requester(episodeCardKey(it)).requestFocus() }
                }
            }
            .focusGroup(),
    ) {
        CompositionLocalProvider(LocalArtworkShown provides artwork) {
            CardRow(state = cards) {
                items(episodes, key = { it.id }, contentType = { "episode" }) { episode ->
                    EpisodeCard(
                        episode = episode,
                        lines = lines,
                        current = episode.id == currentId,
                        stops = stops,
                        onPlay = { navigator.push(Screen.Player(episode.id)) },
                        onOpen = { if (episode.id != currentId) navigator.push(Screen.Work(episode.id)) },
                        modifier = Modifier
                            .focusProperties { this.up = up() }
                            .focusKey(memory, episodeCardKey(episode.id))
                            .onFocusChanged { if (it.isFocused) last = episode.id },
                    )
                }
            }
        }
    }
}

/** A season's cards while they're read, where they will be. */
@Composable
fun EpisodeCardRowSkeleton(modifier: Modifier = Modifier) {
    Row(modifier, horizontalArrangement = Arrangement.spacedBy(CardGap)) {
        repeat(EpisodeCardsAcross) { SkeletonCard(CardShape.Landscape, EpisodeCardWidth) }
    }
}

/** How far a card's text stands under its thumbnail. */
private val CaptionTop = 7.dp

/** How far the lit fill behind a card's text stands clear of it, across and down. */
private val LitAcross = 10.5.dp
private val LitDown = 3.5.dp

@Composable
private fun EpisodeCard(
    episode: WorkSummary,
    lines: Int,
    current: Boolean,
    stops: EpisodeStops,
    onPlay: () -> Unit,
    onOpen: () -> Unit,
    modifier: Modifier,
) {
    val menu = LocalWorkMenu.current
    val focused = remember { mutableStateOf(false) }
    val focus = rememberCardFocus(rests = { stops.onText })
    Column(Modifier.width(EpisodeCardWidth).raisedOnFocus()) {
        CardSurface(
            onClick = { if (okOpensPage(stops.onText, episode.available)) onOpen() else onPlay() },
            onLongClick = { menu(episode) },
            modifier = modifier
                .onPreviewKeyEvent { event ->
                    if (event.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                    val stop = stopAfterArrow(event.key.nativeKeyCode, stops.onText) ?: return@onPreviewKeyEvent false
                    stops.onText = stop
                    focus.settle()
                    true
                }
                .onKeyEvent { event ->
                    if (event.key.nativeKeyCode !in PlayKeys || !episode.available) return@onKeyEvent false
                    if (event.type == KeyEventType.KeyUp) onPlay()
                    true
                }
                .menuKey(episode, menu)
                .onFocusChanged { focused.value = it.isFocused }
                .testTag("episode:${episode.title}")
                .aspectRatio(CardShape.Landscape.aspect),
            focus = focus,
        ) {
            ArtworkImage(cardImage(episode, CardShape.Landscape), episode.title, EpisodeCardWidth)
            CardOverlays(episode, CardShape.Landscape, showCode = false)
            if (current) CurrentMarks()
            if (episode.available) PlayMark { focused.value && !stops.onText }
        }
        Column(Modifier.cardCaption(focus)) {
            Spacer(Modifier.height(CaptionTop))
            EpisodeText(episode, current, lines) { focused.value && stops.onText }
        }
    }
}

/** The page's own episode: ringed in the accent, inside the card so that it grows with it, and tagged. */
@Composable
private fun BoxScope.CurrentMarks() {
    val colors = Kivo.colors
    Box(Modifier.matchParentSize().border(2.dp, colors.accent, RoundedCornerShape(KivoTokens.Radius.md)))
    Text(
        stringResource(R.string.viewing),
        style = TvType.smallStrong,
        color = colors.text,
        modifier = Modifier
            .align(Alignment.BottomStart)
            .padding(start = 7.dp, bottom = 10.5.dp)
            .background(colors.bg, RoundedCornerShape(KivoTokens.Radius.sm))
            .padding(horizontal = 7.dp),
    )
}

/** The play mark over a focused thumbnail, drawn or not as [shown] says, as only the focused card's changes. */
@Composable
private fun BoxScope.PlayMark(shown: () -> Boolean) {
    val colors = Kivo.colors
    Box(
        Modifier
            .align(Alignment.Center)
            .size(56.dp)
            .drawWithContent { if (shown()) drawContent() }
            .background(colors.scrim, RoundedCornerShape(KivoTokens.Radius.full)),
        contentAlignment = Alignment.Center,
    ) {
        KivoIconView(KivoIcon.Play, size = 24.dp, tint = colors.text)
    }
}

/**
 * "4. Reload", then the runtime, or the time left in the accent, before up to
 * [lines] lines of synopsis. While [lit], the text stop, it's dark on a flat
 * white fill. Its colours are given as it's drawn and the fill is drawn behind
 * it, so lighting it costs one card's text a draw.
 */
@Composable
private fun EpisodeText(episode: WorkSummary, current: Boolean, lines: Int, lit: () -> Boolean) {
    val colors = Kivo.colors
    val light = KivoTokens.light
    val e = episode.episode
    val position = episode.userState?.positionMs
    val duration = episode.durationMs
    val time = when {
        duration == null -> null
        position != null -> display.timeLeft((duration - position).toLong())
        else -> display.runtime(duration.toLong())
    }
    val synopsis = remember(e?.overview) { synopsisLine(e?.overview) }
    val timeStyle = remember { TvType.small.copy(fontWeight = KivoTokens.FontWeights.medium) }
    // The time sits at the line's start in its own colours, over its own invisible place in the running text.
    val line = remember(time, synopsis) {
        buildAnnotatedString {
            if (time != null) {
                withStyle(SpanStyle(color = Color.Transparent, fontWeight = KivoTokens.FontWeights.medium)) { append(time) }
                if (synopsis != null) append(" · ")
            }
            if (synopsis != null) append(synopsis)
        }
    }
    val number = e?.let { if (it.endNumber != null && it.endNumber != it.number) "${it.number}–${it.endNumber}" else "${it.number}" }
    Column(
        Modifier
            .fillMaxWidth()
            .drawBehind {
                if (!lit()) return@drawBehind
                val across = LitAcross.toPx()
                val down = LitDown.toPx()
                drawRoundRect(
                    Lit.container,
                    topLeft = Offset(-across, -down),
                    size = Size(size.width + across * 2, size.height + down * 2),
                    cornerRadius = CornerRadius(KivoTokens.Radius.md.toPx()),
                )
            },
        verticalArrangement = Arrangement.spacedBy(3.5.dp),
    ) {
        BasicText(
            if (number != null) "$number. ${episode.title}" else episode.title,
            style = TvType.smallStrong,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            color = ColorProducer {
                when {
                    lit() -> if (current) light.accent else light.text
                    current -> colors.accent
                    else -> colors.text
                }
            },
        )
        Box {
            BasicText(
                line,
                style = TvType.small,
                maxLines = lines,
                minLines = lines,
                overflow = TextOverflow.Ellipsis,
                color = ColorProducer { if (lit()) light.text else colors.textMuted },
            )
            if (time != null) {
                BasicText(
                    time,
                    style = timeStyle,
                    maxLines = 1,
                    color = if (position != null) {
                        ColorProducer { if (lit()) light.accent else colors.accent }
                    } else {
                        ColorProducer { if (lit()) light.text else colors.text }
                    },
                )
            }
        }
    }
}
