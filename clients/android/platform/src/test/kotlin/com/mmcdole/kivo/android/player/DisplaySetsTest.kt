package com.mmcdole.kivo.android.player

import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals

class DisplaySetsTest {
    /** A `.sup` segment: "PG", its presentation time, a decoding time, then its type, size and data. */
    private fun segment(pts: Long, type: Int, vararg data: Int): ByteArray {
        val head = byteArrayOf(
            'P'.code.toByte(), 'G'.code.toByte(),
            (pts shr 24).toByte(), (pts shr 16).toByte(), (pts shr 8).toByte(), pts.toByte(),
            0, 0, 0, 0,
            type.toByte(), (data.size shr 8).toByte(), data.size.toByte(),
        )
        return head + data.map { it.toByte() }.toByteArray()
    }

    @Test
    fun eachSetRunsToItsEndAndShowsAtItsFirstSegmentsTime() {
        val file = segment(90_000, 0x16, 1, 2, 3) + segment(90_000, 0x14, 4) + segment(90_000, 0x80) +
            segment(270_000, 0x16, 5) + segment(270_000, 0x80) +
            segment(360_000, 0x16, 6) // cut short: no end
        val sets = displaySets(file)
        assertEquals(listOf(1_000_000L, 3_000_000L), sets.map { it.startUs })
        assertContentEquals(byteArrayOf(0x16, 0, 3, 1, 2, 3, 0x14, 0, 1, 4, 0x80.toByte(), 0, 0), sets[0].segments, "as Matroska stores a sample")
        assertContentEquals(byteArrayOf(0x16, 0, 1, 5, 0x80.toByte(), 0, 0), sets[1].segments)
    }

    @Test
    fun aTimeBeyondTwoToTheThirtyOneIsUnsigned() {
        val sets = displaySets(segment(0xF000_0000, 0x16) + segment(0xF000_0000, 0x80))
        assertEquals(0xF000_0000 * 1_000_000 / 90_000, sets.single().startUs)
    }

    @Test
    fun aSegmentLongerThanTheFileEndsIt() {
        val whole = segment(0, 0x16, 1, 2) + segment(0, 0x80)
        assertEquals(1, displaySets(whole).size)
        assertEquals(0, displaySets(whole.copyOf(whole.size - 1)).size)
        assertEquals(0, displaySets("not a sup".encodeToByteArray()).size)
    }
}
