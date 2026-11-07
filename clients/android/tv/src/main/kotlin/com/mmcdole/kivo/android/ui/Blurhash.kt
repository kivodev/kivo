package com.mmcdole.kivo.android.ui

import kotlin.math.PI
import kotlin.math.cos
import kotlin.math.pow
import kotlin.math.withSign

/**
 * Decodes a blurhash (blurha.sh), the placeholder the server computes for
 * each image so artwork never loads over a grey box, into
 * ARGB pixels; null for a malformed hash.
 */
object Blurhash {
    private const val DIGITS = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~"

    fun decode(hash: String, width: Int, height: Int): IntArray? {
        if (hash.length < 6) return null
        val size = decode83(hash, 0, 1) ?: return null
        val componentsY = size / 9 + 1
        val componentsX = size % 9 + 1
        if (hash.length != 4 + 2 * componentsX * componentsY) return null
        val maximum = ((decode83(hash, 1, 2) ?: return null) + 1) / 166f
        val colors = ArrayList<FloatArray>(componentsX * componentsY)
        val dc = decode83(hash, 2, 6) ?: return null
        colors += floatArrayOf(toLinear(dc shr 16), toLinear((dc shr 8) and 255), toLinear(dc and 255))
        for (i in 1 until componentsX * componentsY) {
            val ac = decode83(hash, 4 + i * 2, 6 + i * 2) ?: return null
            colors += floatArrayOf(
                signedSquare(ac / (19 * 19) - 9) * maximum,
                signedSquare((ac / 19) % 19 - 9) * maximum,
                signedSquare(ac % 19 - 9) * maximum,
            )
        }
        // The basis functions, once per column and row rather than once per pixel and component.
        val cosX = FloatArray(width * componentsX) { cos(PI * (it / componentsX) * (it % componentsX) / width).toFloat() }
        val cosY = FloatArray(height * componentsY) { cos(PI * (it / componentsY) * (it % componentsY) / height).toFloat() }
        val pixels = IntArray(width * height)
        for (y in 0 until height) {
            for (x in 0 until width) {
                var r = 0f
                var g = 0f
                var b = 0f
                for (j in 0 until componentsY) {
                    val basisY = cosY[y * componentsY + j]
                    for (i in 0 until componentsX) {
                        val basis = cosX[x * componentsX + i] * basisY
                        val color = colors[i + j * componentsX]
                        r += color[0] * basis
                        g += color[1] * basis
                        b += color[2] * basis
                    }
                }
                pixels[x + y * width] = (0xFF shl 24) or (toSrgb(r) shl 16) or (toSrgb(g) shl 8) or toSrgb(b)
            }
        }
        return pixels
    }

    private fun decode83(text: String, from: Int, to: Int): Int? {
        var value = 0
        for (i in from until to) {
            val digit = DIGITS.indexOf(text[i])
            if (digit < 0) return null
            value = value * 83 + digit
        }
        return value
    }

    private fun signedSquare(quantised: Int): Float = (quantised / 9f).let { (it * it).withSign(it) }

    private fun toLinear(value: Int): Float {
        val v = value / 255f
        return if (v <= 0.04045f) v / 12.92f else ((v + 0.055f) / 1.055f).pow(2.4f)
    }

    private fun toSrgb(value: Float): Int {
        val v = value.coerceIn(0f, 1f)
        val srgb = if (v <= 0.0031308f) v * 12.92f else 1.055f * v.pow(1 / 2.4f) - 0.055f
        return (srgb * 255 + 0.5f).toInt().coerceIn(0, 255)
    }
}
