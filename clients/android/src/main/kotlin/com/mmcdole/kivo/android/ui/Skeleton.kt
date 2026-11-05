package com.mmcdole.kivo.android.ui

import android.os.SystemClock
import androidx.compose.animation.Crossfade
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.mmcdole.kivo.android.theme.Kivo
import com.mmcdole.kivo.android.theme.KivoTokens
import com.mmcdole.kivo.android.theme.TvType
import kotlinx.coroutines.delay

/** How long a load goes before its skeleton shows, and how long the skeleton then stays at least. */
private const val SKELETON_DELAY_MS = 300L

/** What arrives fades in: 200 ms, easing out; nothing slides. */
fun <T> fadeSpec() = tween<T>(KivoTokens.Duration.fadeMs, easing = KivoTokens.Ease.out)

private enum class Stage { Waiting, Skeleton, Content }

/**
 * A screen's first load: nothing for its first 300 ms, so a
 * quick load goes straight to the content, then [skeleton], which once
 * shown stays at least 300 ms so it doesn't flicker; the content fades in.
 * Once there it stays: a refresh changes [value] in place. A value already
 * at hand, such as a grid seen lately, shows at once.
 */
@Composable
fun <T : Any> SkeletonGate(value: T?, skeleton: @Composable () -> Unit, content: @Composable (T) -> Unit) {
    var skeletonSince by remember { mutableStateOf<Long?>(null) }
    var last by remember { mutableStateOf(value) }
    if (value != null) last = value
    val loaded = value != null
    LaunchedEffect(loaded) {
        val since = skeletonSince
        if (!loaded && since == null) {
            delay(SKELETON_DELAY_MS)
            skeletonSince = SystemClock.uptimeMillis()
        } else if (loaded && since != null) {
            delay(since + SKELETON_DELAY_MS - SystemClock.uptimeMillis())
            skeletonSince = null
        }
    }
    val stage = when {
        skeletonSince != null -> Stage.Skeleton
        loaded -> Stage.Content
        else -> Stage.Waiting
    }
    Crossfade(stage, animationSpec = fadeSpec(), label = "load") { shown ->
        when (shown) {
            Stage.Waiting -> Box(Modifier.fillMaxSize())
            Stage.Skeleton -> skeleton()
            Stage.Content -> last?.let { content(it) }
        }
    }
}

/** A placeholder shape in the layout of what's loading, so the screen keeps its shape. */
@Composable
fun SkeletonBlock(modifier: Modifier = Modifier, radius: Dp = KivoTokens.Radius.md) {
    Box(modifier.background(Kivo.colors.placeholder, RoundedCornerShape(radius)))
}

/** A bar in a line of text in [style], [width] wide: a title or a caption still loading. */
@Composable
fun SkeletonLine(width: Dp, style: TextStyle, modifier: Modifier = Modifier) {
    val height = lineHeight(style)
    Box(modifier.height(height)) {
        SkeletonBlock(Modifier.width(width).height(height * 0.6f).align(Alignment.CenterStart), radius = 3.dp)
    }
}

/** A card's skeleton: its artwork's shape and its two caption lines, as tall as the card. */
@Composable
fun SkeletonCard(shape: CardShape, width: Dp, modifier: Modifier = Modifier) {
    Column(modifier.width(width)) {
        SkeletonBlock(Modifier.fillMaxWidth().aspectRatio(shape.aspect))
        Spacer(Modifier.height(CaptionGap))
        SkeletonLine(width * 0.7f, TvType.smallStrong)
        Spacer(Modifier.height(lineHeight(TvType.small)))
    }
}

/** A row's skeleton: its title and as many cards as fit, laid out as a row of cards is. */
@Composable
fun SkeletonRow(shape: CardShape, width: Dp = cardWidth(shape), count: Int = if (shape == CardShape.Landscape) 4 else 6) {
    Column {
        SkeletonLine(160.dp, TvType.large, Modifier.padding(bottom = 12.dp))
        Row(Modifier.padding(vertical = FocusBleed), horizontalArrangement = Arrangement.spacedBy(CardGap)) {
            repeat(count) { SkeletonCard(shape, width) }
        }
    }
}

/** A detail page's hero while it loads: the title, its line, the buttons and the overview, where they will be. */
@Composable
fun SkeletonHero(modifier: Modifier = Modifier) {
    Column(modifier, verticalArrangement = Arrangement.spacedBy(Kivo.space(5))) {
        Column(verticalArrangement = Arrangement.spacedBy(Kivo.space(2))) {
            SkeletonLine(360.dp, TvType.title)
            SkeletonLine(240.dp, TvType.body)
        }
        Row(horizontalArrangement = Arrangement.spacedBy(Kivo.space(3))) {
            SkeletonBlock(Modifier.width(150.dp).height(44.dp))
            SkeletonBlock(Modifier.size(44.dp), radius = KivoTokens.Radius.full)
            SkeletonBlock(Modifier.size(44.dp), radius = KivoTokens.Radius.full)
        }
        Column {
            SkeletonLine(560.dp, TvType.body)
            SkeletonLine(520.dp, TvType.body)
            SkeletonLine(300.dp, TvType.body)
        }
    }
}

@Composable
private fun lineHeight(style: TextStyle): Dp =
    with(LocalDensity.current) { style.lineHeight.toDp() }
