package com.mmcdole.kivo.core.player

/**
 * A version's parts on one timeline: every position the player reports or
 * seeks to is on it, and a multi-part file plays each part in turn, offset by
 * the parts before it.
 */
class PartTimeline(private val durationsMs: List<Long>) {
    private val offsets = durationsMs.runningFold(0L) { sum, duration -> sum + duration }

    val durationMs: Long get() = offsets.last()

    /** The part a position falls in and the offset into it; past the end, the last part's end. */
    fun locate(positionMs: Long): PartPosition {
        if (durationsMs.isEmpty()) return PartPosition(0, 0)
        val clamped = positionMs.coerceIn(0, durationMs)
        val part = (offsets.indexOfLast { it <= clamped }).coerceAtMost(durationsMs.lastIndex)
        return PartPosition(part, clamped - offsets[part])
    }

    /** The timeline position of an offset into a part. */
    fun position(part: Int, offsetMs: Long): Long = offsets[part.coerceIn(0, durationsMs.size)] + offsetMs
}

/** A position in one part of a version: the part's index and the offset into it. */
data class PartPosition(val part: Int, val offsetMs: Long)
