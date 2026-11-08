package com.mmcdole.kivo.android.mobile.ui

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.wrapContentSize
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.mmcdole.kivo.android.mobile.theme.LocalKivo
import com.mmcdole.kivo.android.mobile.theme.space
import com.mmcdole.kivo.api.models.CollectionSummary
import com.mmcdole.kivo.api.models.ImageRef
import com.mmcdole.kivo.api.models.WorkSummary

/** A poster's corners, as the design has them. */
val CardCorners = RoundedCornerShape(10.dp)

/** A card's widths in a row: landscape and the rest. */
val RowLandscapeWidth = 260.dp
val RowPosterWidth = 116.dp

fun rowCardWidth(shape: CardShape): Dp = if (shape == CardShape.Landscape) RowLandscapeWidth else RowPosterWidth

/** The artwork a card of [shape] shows of [work]: a frame for landscape, else the poster. */
fun cardImage(work: WorkSummary, shape: CardShape): ImageRef? = with(work.images) {
    if (shape == CardShape.Landscape) still ?: backdrop ?: poster else poster ?: still
}

/** A tap and a long press, the long press with a haptic tick. */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun Modifier.tapAndHold(onClick: () -> Unit, onLongClick: (() -> Unit)?): Modifier {
    val haptics = LocalHapticFeedback.current
    return combinedClickable(
        onClick = onClick,
        onLongClick = onLongClick?.let { hold ->
            {
                haptics.performHapticFeedback(HapticFeedbackType.LongPress)
                hold()
            }
        },
    )
}

/**
 * A work's card: artwork in [shape], [width] wide, with a played check or
 * unplayed count and a progress bar, then its title and, in [captionLines]
 * of two, the line under it. Null [work] is a placeholder while its page
 * is read.
 */
@Composable
fun WorkCard(
    work: WorkSummary?,
    shape: CardShape,
    width: Dp,
    onClick: () -> Unit,
    onLongClick: (() -> Unit)?,
    modifier: Modifier = Modifier,
    captionLines: Int = 2,
) {
    Column(modifier.width(width)) {
        Box(
            Modifier
                .fillMaxWidth()
                .aspectRatio(shape.aspect)
                .clip(CardCorners)
                .then(if (work != null) Modifier.tapAndHold(onClick, onLongClick).testTag("card:${work.title}") else Modifier),
        ) {
            if (work == null) {
                Box(Modifier.matchParentSize().background(LocalKivo.current.placeholder))
            } else {
                Artwork(cardImage(work, shape), width, Modifier.matchParentSize())
                Mark(work, Modifier.align(Alignment.TopEnd).padding(6.dp))
                progressOf(work)?.let { ProgressBar(it, Modifier.align(Alignment.BottomCenter)) }
            }
        }
        Caption(work?.let(::cardTitle), work?.let(::cardCaption), captionLines)
    }
}

/** A card's caption under its artwork: the title, and the line under it, which keeps its room. */
@Composable
fun Caption(title: String?, detail: String?, lines: Int) {
    Text(
        title.orEmpty(),
        Modifier.padding(top = space(2)),
        style = MaterialTheme.typography.bodyMedium,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
    )
    if (lines > 1) {
        Text(
            detail.orEmpty(),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

/** A played check, or a series' or season's unplayed count. */
@Composable
private fun Mark(work: WorkSummary, modifier: Modifier) {
    val unplayed = work.series?.unplayedCount ?: work.season?.unplayedCount
    val played = work.userState?.played == true
    val badge = LocalKivo.current.badge
    when {
        unplayed != null && unplayed > 0 -> Text(
            unplayed.toString(),
            modifier.background(MaterialTheme.colorScheme.primary, CircleShape).padding(horizontal = 7.dp, vertical = 1.dp),
            color = MaterialTheme.colorScheme.onPrimary,
            style = MaterialTheme.typography.labelSmall,
        )
        played || unplayed == 0 -> Icon(
            Icons.Default.Check,
            null,
            modifier.background(badge, CircleShape).padding(3.dp).size(14.dp),
            tint = Color.White,
        )
    }
}

/** A 4 dp bar of how far in a work is, along a card's bottom edge. */
@Composable
fun ProgressBar(progress: Float, modifier: Modifier = Modifier) {
    Box(modifier.fillMaxWidth().height(4.dp).background(LocalKivo.current.progressTrack)) {
        Box(Modifier.fillMaxHeight().fillMaxWidth(progress).background(MaterialTheme.colorScheme.primary))
    }
}

/** A collection's card: its poster, name and count. */
@Composable
fun CollectionCard(collection: CollectionSummary?, width: Dp, onClick: () -> Unit, modifier: Modifier = Modifier) {
    Column(modifier.width(width)) {
        Box(
            Modifier
                .fillMaxWidth()
                .aspectRatio(CardShape.Poster.aspect)
                .clip(CardCorners)
                .then(if (collection != null) Modifier.tapAndHold(onClick, null) else Modifier),
        ) {
            Artwork(collection?.images?.poster, width, Modifier.matchParentSize())
        }
        Caption(collection?.name, collection?.workCount?.toString(), 1)
    }
}

/** A square or round picture with a name and line under it: an author, a book series or a credit. */
@Composable
fun NamedCard(
    name: String?,
    detail: String?,
    image: ImageRef?,
    width: Dp,
    round: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(modifier.width(width), horizontalAlignment = if (round) Alignment.CenterHorizontally else Alignment.Start) {
        Box(
            Modifier
                .fillMaxWidth()
                .aspectRatio(1f)
                .clip(if (round) CircleShape else CardCorners)
                .then(if (name != null) Modifier.tapAndHold(onClick, null) else Modifier),
        ) {
            Artwork(image, width, Modifier.matchParentSize())
            if (image == null && name != null) {
                Text(
                    name.take(1).uppercase(),
                    Modifier.align(Alignment.Center).wrapContentSize(),
                    style = MaterialTheme.typography.headlineSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        Text(name.orEmpty(), Modifier.padding(top = space(2)), style = MaterialTheme.typography.bodyMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
        Text(
            detail.orEmpty(),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}
