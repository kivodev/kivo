package com.mmcdole.kivo.android.ui

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull

class BlurhashTest {
    private val digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~"

    private fun encode83(value: Int, length: Int): String {
        var rest = value
        val out = CharArray(length)
        for (i in length - 1 downTo 0) {
            out[i] = digits[rest % 83]
            rest /= 83
        }
        return String(out)
    }

    @Test
    fun aHashOfOneComponentIsItsColourEverywhere() {
        // One component (size 0), any maximum, and the colour #3366cc.
        val hash = encode83(0, 1) + encode83(0, 1) + encode83(0x3366CC, 4)
        val pixels = assertNotNull(Blurhash.decode(hash, 4, 3))
        assertEquals(12, pixels.size)
        for (pixel in pixels) assertEquals(0xFF3366CC.toInt(), pixel)
    }

    @Test
    fun aRealHashDecodesToOpaquePixels() {
        val pixels = assertNotNull(Blurhash.decode("LEHV6nWB2yk8pyo0adR*.7kCMdnj", 32, 32))
        assertEquals(32 * 32, pixels.size)
        for (pixel in pixels) assertEquals(0xFF, pixel ushr 24)
    }

    @Test
    fun aMalformedHashIsNull() {
        assertNull(Blurhash.decode("LEHV6n", 8, 8))
        assertNull(Blurhash.decode("LEHV6nWB2yk8pyo0adR*.7kCMdn", 8, 8))
        assertNull(Blurhash.decode("L\"HV6nWB2yk8pyo0adR*.7kCMdnj", 8, 8))
    }
}
