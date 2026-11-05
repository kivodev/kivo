package com.mmcdole.kivo.core.books

import com.mmcdole.kivo.api.models.Chapter
import com.mmcdole.kivo.api.models.PlayStarted
import com.mmcdole.kivo.api.models.Version
import com.mmcdole.kivo.core.player.PartTimeline

/**
 * A book's chapters on its version's timeline: the seek bar spans the chapter
 * playing, and previous and next move by chapter, on the lock screen too.
 * They're the server's, which already puts every part's chapters on the
 * timeline; with none, a book of several parts is a chapter per part, and a
 * single file one chapter, the whole book.
 */
class BookChapters(chapters: List<Chapter>, partDurationsMs: List<Long>) {
    /** A play's chapters, on the timeline of the parts it plays. */
    constructor(play: PlayStarted) : this(play.chapters, play.parts.map { it.durationMs.toLong() })

    /** A version's chapters, for its book's page to list before anything plays. */
    constructor(version: Version) : this(version.chapters.orEmpty(), version.parts.map { it.durationMs.toLong() })

    private val parts = PartTimeline(partDurationsMs)

    /** The book's length: the parts', or the chapters' end without parts. */
    val durationMs: Long = if (partDurationsMs.isNotEmpty()) parts.durationMs else chapters.maxOfOrNull { it.endMs.toLong() } ?: 0

    val list: List<Chapter> = when {
        chapters.isNotEmpty() -> chapters.sortedBy { it.startMs }
        partDurationsMs.size > 1 -> partDurationsMs.indices.map { i ->
            Chapter(parts.position(i, 0).toInt(), parts.position(i + 1, 0).toInt())
        }
        else -> listOf(Chapter(0, durationMs.toInt()))
    }

    val count: Int get() = list.size

    /** The index of the chapter at a position: the last to start at or before it. */
    fun indexAt(positionMs: Long): Int = list.indexOfLast { it.startMs <= positionMs }.coerceAtLeast(0)

    /** Where a chapter starts. */
    fun startMs(index: Int): Long = list[index.coerceIn(0, list.lastIndex)].startMs.toLong()

    /** Where a chapter ends: the next one's start, else its own end, else the book's. */
    fun endMs(index: Int): Long {
        val i = index.coerceIn(0, list.lastIndex)
        return list.getOrNull(i + 1)?.startMs?.toLong() ?: maxOf(list[i].endMs.toLong(), durationMs)
    }

    /** Where Previous goes: the chapter's start, or the start of the chapter before within 3 seconds of it. */
    fun previousMs(positionMs: Long): Long {
        val i = indexAt(positionMs)
        val start = startMs(i)
        return if (positionMs - start > RESTART_WITHIN_MS || i == 0) start else startMs(i - 1)
    }

    /** Where Next goes: the next chapter's start; null in the last. */
    fun nextMs(positionMs: Long): Long? = list.getOrNull(indexAt(positionMs) + 1)?.startMs?.toLong()

    /** Where a position is in its chapter and in the book, for the seek bar, the progress line and the time left. */
    fun progress(positionMs: Long): BookProgress {
        val position = positionMs.coerceIn(0, durationMs)
        val i = indexAt(position)
        val start = startMs(i)
        val end = endMs(i)
        return BookProgress(
            chapterIndex = i,
            chapterCount = list.size,
            chapterTitle = list[i].title,
            chapterStartMs = start,
            chapterDurationMs = end - start,
            chapterElapsedMs = (position - start).coerceIn(0, end - start),
            positionMs = position,
            durationMs = durationMs,
            hasNext = i < list.lastIndex,
        )
    }
}

/** Previous chapter within this long of a chapter's start goes to the chapter before. */
internal const val RESTART_WITHIN_MS = 3_000L

/**
 * Where a book is, as values the platform formats: the chapter playing as
 * "Chapter 12 of 56", or by its [chapterTitle], with the time played in it
 * and left in it either side of its seek bar; the whole book's progress line
 * and the time left in it.
 */
data class BookProgress(
    /** From 0; "Chapter 12" is index 11. */
    val chapterIndex: Int,
    val chapterCount: Int,
    val chapterTitle: String?,
    val chapterStartMs: Long,
    val chapterDurationMs: Long,
    val chapterElapsedMs: Long,
    val positionMs: Long,
    val durationMs: Long,
    /** Whether there's a chapter after this one, for Next. */
    val hasNext: Boolean,
) {
    val chapterRemainingMs: Long get() = chapterDurationMs - chapterElapsedMs

    /** How far through the chapter, 0 to 1. */
    val chapterFraction: Double get() = if (chapterDurationMs > 0) chapterElapsedMs.toDouble() / chapterDurationMs else 0.0

    val remainingMs: Long get() = (durationMs - positionMs).coerceAtLeast(0)

    /** How far through the book, 0 to 1. */
    val fraction: Double get() = if (durationMs > 0) positionMs.toDouble() / durationMs else 0.0
}
