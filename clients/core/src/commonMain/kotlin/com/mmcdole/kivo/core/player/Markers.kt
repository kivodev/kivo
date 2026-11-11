package com.mmcdole.kivo.core.player

import com.mmcdole.kivo.api.models.Chapter
import com.mmcdole.kivo.api.models.PlayStarted
import kotlin.time.Duration.Companion.seconds
import com.mmcdole.kivo.api.models.Marker as ServerMarker

// Skip markers: the parts of a video the server marks, an intro, a recap or
// credits, which the player offers to skip.

/** Back 10 seconds and forward 30, with no setting. */
const val SKIP_BACK_MS = 10_000L
const val SKIP_FORWARD_MS = 30_000L

/** The chapter at a position. */
fun chapterAt(chapters: List<Chapter>, positionMs: Long): Chapter? =
    chapters.firstOrNull { positionMs >= it.startMs && positionMs < it.endMs }

/** How long Skip shows on its own after playback enters a marker, in wall-clock time. */
internal val SKIP_SHOWS = 10.seconds

/** A marker shorter than this has no Skip. */
internal const val MARKER_MIN_MS = 3_000L

/** What a marker skips, which names its button. */
enum class MarkerKind {
    Intro,
    Recap,
    Credits,
}

/**
 * A part of the video a player offers to skip, on the version's timeline.
 * [final] credits have nothing after them but, perhaps, a next episode's
 * preview, so up next appears at their start.
 */
data class SkipMarker(val kind: MarkerKind, val startMs: Long, val endMs: Long, val final: Boolean = false)

/**
 * A play's markers: the server's, leaving out kinds this app doesn't know;
 * none shorter than [MARKER_MIN_MS].
 */
internal fun markersOf(play: PlayStarted): List<SkipMarker> = markersOf(play.markers)

internal fun markersOf(markers: List<ServerMarker>?): List<SkipMarker> =
    markers.orEmpty()
        .mapNotNull { m -> kindOf(m.kind)?.let { SkipMarker(it, m.startMs.toLong(), m.endMs.toLong(), m.final == true) } }
        .filter { it.endMs - it.startMs >= MARKER_MIN_MS }

private fun kindOf(kind: String?): MarkerKind? = when (kind) {
    "intro" -> MarkerKind.Intro
    "recap" -> MarkerKind.Recap
    "credits" -> MarkerKind.Credits
    else -> null
}

/** The marker at a position. */
internal fun markerAt(markers: List<SkipMarker>, positionMs: Long): SkipMarker? =
    markers.firstOrNull { positionMs >= it.startMs && positionMs < it.endMs }

/** The credits up next appears at: the last credits, when they're final credits. */
internal fun upNextCredits(markers: List<SkipMarker>): SkipMarker? =
    markers.lastOrNull { it.kind == MarkerKind.Credits }?.takeIf { it.final }

/**
 * The marker whose Skip button shows. It appears as playback enters a
 * marker and hides on its own [SKIP_SHOWS] later, playing or paused, when
 * the player's clock says so ([expire]); while playback stays in the marker
 * it shows with the controls, and it goes at the marker's end. Skip hides
 * it for that marker, and a seek back into the marker shows it again. Skip
 * Credits gives way to up next.
 */
internal class SkipShow {
    private var startMs: Long? = null
    private var expired = false
    private var skipped = false

    /** Whether the last look entered a marker, which starts its [SKIP_SHOWS]. */
    var entered = false
        private set

    /** The marker whose Skip shows at [positionMs]. */
    fun at(markers: List<SkipMarker>, positionMs: Long, controlsShowing: Boolean, upNextShowing: Boolean): SkipMarker? {
        val marker = markerAt(markers, positionMs)
        entered = marker != null && marker.startMs != startMs
        if (marker?.startMs != startMs) {
            startMs = marker?.startMs
            expired = false
            skipped = false
        }
        if (marker == null || skipped || marker.kind == MarkerKind.Credits && upNextShowing) return null
        return marker.takeIf { controlsShowing || !expired }
    }

    /** [SKIP_SHOWS] passed since playback entered the marker: Skip shows with the controls only. */
    fun expire() {
        expired = true
    }

    /** Skip was pressed: the button stays away until playback leaves the marker. */
    fun skip() {
        skipped = true
    }
}
