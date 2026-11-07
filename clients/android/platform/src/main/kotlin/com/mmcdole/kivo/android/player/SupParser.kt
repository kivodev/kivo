package com.mmcdole.kivo.android.player

import androidx.annotation.OptIn
import androidx.media3.common.C
import androidx.media3.common.Format
import androidx.media3.common.MimeTypes
import androidx.media3.common.util.Consumer
import androidx.media3.common.util.UnstableApi
import androidx.media3.extractor.text.CuesWithTiming
import androidx.media3.extractor.text.SubtitleParser
import androidx.media3.extractor.text.pgs.PgsParser

/**
 * The type a PGS file is given to Media3 as: a `.sup` file as the server
 * keeps it, which [SupParser] reads. Media3's own `application/pgs` is one
 * display set without its timing, as Matroska stores each.
 */
const val SUP_MIME = "application/x-kivo-sup"

/**
 * A `.sup` file, read whole: each display set at its own time, its pictures
 * drawn by Media3's PGS parser, and each replacing the one before, as a
 * set with no picture clears the screen.
 */
@OptIn(UnstableApi::class)
class SupParser : SubtitleParser {
    private val pgs = PgsParser()

    override fun getCueReplacementBehavior(): Int = PgsParser.CUE_REPLACEMENT_BEHAVIOR

    override fun parse(data: ByteArray, offset: Int, length: Int, outputOptions: SubtitleParser.OutputOptions, output: Consumer<CuesWithTiming>) {
        val sets = displaySets(data, offset, length)
        val from = outputOptions.startTimeUs.takeIf { it != C.TIME_UNSET } ?: Long.MIN_VALUE
        // From the start time first; the sets before it after, when asked for.
        val order = sets.indices.filter { sets[it].startUs >= from } + if (outputOptions.outputAllCues) sets.indices.filter { sets[it].startUs < from } else emptyList()
        for (i in order) {
            val set = sets[i]
            val durationUs = sets.getOrNull(i + 1)?.let { it.startUs - set.startUs } ?: C.TIME_UNSET
            pgs.parse(set.segments, 0, set.segments.size, SubtitleParser.OutputOptions.allCues()) { parsed ->
                output.accept(CuesWithTiming(parsed.cues, set.startUs, durationUs))
            }
        }
    }

    /** Media3's subtitle parsers, with [SupParser] for [SUP_MIME]. */
    class Factory(private val others: SubtitleParser.Factory) : SubtitleParser.Factory {
        init {
            SupMime
        }

        override fun supportsFormat(format: Format): Boolean = format.sampleMimeType == SUP_MIME || others.supportsFormat(format)

        override fun getCueReplacementBehavior(format: Format): Int =
            if (format.sampleMimeType == SUP_MIME) PgsParser.CUE_REPLACEMENT_BEHAVIOR else others.getCueReplacementBehavior(format)

        override fun create(format: Format): SubtitleParser = if (format.sampleMimeType == SUP_MIME) SupParser() else others.create(format)
    }

}

/** [SUP_MIME] known to Media3 as a text type, once. */
@OptIn(UnstableApi::class)
private object SupMime {
    init {
        MimeTypes.registerCustomMimeType(SUP_MIME, "sup", C.TRACK_TYPE_TEXT)
    }
}

/** A display set of a `.sup` file: its segments as Matroska stores a PGS sample, and when it shows. */
class DisplaySet(val startUs: Long, val segments: ByteArray)

/**
 * The display sets of a `.sup` file: each segment is "PG", its 90 kHz
 * presentation and decoding times, then the segment's type, size and data;
 * a set runs to its end segment and shows at its first segment's
 * presentation time. A file cut short keeps the sets it completed.
 */
fun displaySets(data: ByteArray, offset: Int = 0, length: Int = data.size): List<DisplaySet> {
    val out = mutableListOf<DisplaySet>()
    val end = offset + length
    var at = offset
    var current: java.io.ByteArrayOutputStream? = null
    var startUs = 0L
    while (at + HEADER <= end && data[at] == 'P'.code.toByte() && data[at + 1] == 'G'.code.toByte()) {
        val pts = u32(data, at + 2)
        val type = data[at + 10].toInt() and 0xff
        val size = ((data[at + 11].toInt() and 0xff) shl 8) or (data[at + 12].toInt() and 0xff)
        if (at + HEADER + size > end) break
        val set = current ?: java.io.ByteArrayOutputStream().also {
            current = it
            startUs = pts * 1_000_000 / 90_000
        }
        set.write(data, at + 10, 3 + size)
        at += HEADER + size
        if (type == END_SEGMENT) {
            out += DisplaySet(startUs, set.toByteArray())
            current = null
        }
    }
    return out
}

private const val HEADER = 13
private const val END_SEGMENT = 0x80

private fun u32(b: ByteArray, at: Int): Long =
    ((b[at].toLong() and 0xff) shl 24) or ((b[at + 1].toLong() and 0xff) shl 16) or ((b[at + 2].toLong() and 0xff) shl 8) or (b[at + 3].toLong() and 0xff)
