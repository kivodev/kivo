package com.mmcdole.kivo.core.auth

// Ed25519 signature checking (RFC 8032), after TweetNaCl's crypto_sign_open:
// field elements mod 2^255 - 19 are 16 limbs of 16 bits, points are extended
// coordinates (X, Y, Z, T). It only checks public data, so it needn't run
// in constant time.

/** Whether [signature] is [publicKey]'s Ed25519 signature of [message]. */
internal fun ed25519Verify(publicKey: ByteArray, message: ByteArray, signature: ByteArray): Boolean {
    if (publicKey.size != 32 || signature.size != 64) return false
    val a = point()
    if (!unpackNegative(a, publicKey)) return false
    val r = signature.copyOfRange(0, 32)
    val h = sha512(r + publicKey + message).also(::reduce)
    val p = point()
    scalarMult(p, a, h)
    add(p, scalarBase(signature.copyOfRange(32, 64)))
    return encode(p).contentEquals(r)
}

private fun fe(vararg limbs: Long) = LongArray(16).also { limbs.copyInto(it) }

private fun point() = arrayOf(fe(), fe(1), fe(1), fe())

private val D = fe(0x78a3, 0x1359, 0x4dca, 0x75eb, 0xd8ab, 0x4141, 0x0a4d, 0x0070, 0xe898, 0x7779, 0x4079, 0x8cc7, 0xfe73, 0x2b6f, 0x6cee, 0x5203)
private val D2 = fe(0xf159, 0x26b2, 0x9b94, 0xebd6, 0xb156, 0x8283, 0x149a, 0x00e0, 0xd130, 0xeef3, 0x80f2, 0x198e, 0xfce7, 0x56df, 0xd9dc, 0x2406)
private val BX = fe(0xd51a, 0x8f25, 0x2d60, 0xc956, 0xa7b2, 0x9525, 0xc760, 0x692c, 0xdc5c, 0xfdd6, 0xe231, 0xc0a4, 0x53fe, 0xcd6e, 0x36d3, 0x2169)
private val BY = fe(0x6658, 0x6666, 0x6666, 0x6666, 0x6666, 0x6666, 0x6666, 0x6666, 0x6666, 0x6666, 0x6666, 0x6666, 0x6666, 0x6666, 0x6666, 0x6666)
private val SQRT_M1 = fe(0xa0b0, 0x4a0e, 0x1b27, 0xc4ee, 0xe478, 0xad2f, 0x1806, 0x2f43, 0xd7a7, 0x3dfb, 0x0099, 0x2b4d, 0xdf0b, 0x4fc1, 0x2480, 0x2b83)

/** The group order, little-endian. */
private val L = longArrayOf(
    0xed, 0xd3, 0xf5, 0x5c, 0x1a, 0x63, 0x12, 0x58, 0xd6, 0x9c, 0xf7, 0xa2, 0xde, 0xf9, 0xde, 0x14,
    0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x10,
)

private fun carry(o: LongArray) {
    for (i in 0 until 16) {
        o[i] += 1L shl 16
        val c = o[i] shr 16
        if (i < 15) o[i + 1] += c - 1 else o[0] += 38 * (c - 1)
        o[i] -= c shl 16
    }
}

private fun pack(o: ByteArray, n: LongArray) {
    val t = n.copyOf()
    val m = LongArray(16)
    repeat(3) { carry(t) }
    repeat(2) {
        m[0] = t[0] - 0xffed
        for (i in 1 until 15) {
            m[i] = t[i] - 0xffff - ((m[i - 1] shr 16) and 1)
            m[i - 1] = m[i - 1] and 0xffff
        }
        m[15] = t[15] - 0x7fff - ((m[14] shr 16) and 1)
        m[14] = m[14] and 0xffff
        if ((m[15] shr 16) and 1 == 0L) m.copyInto(t)
    }
    for (i in 0 until 16) {
        o[2 * i] = t[i].toByte()
        o[2 * i + 1] = (t[i] shr 8).toByte()
    }
}

private fun encoded(n: LongArray) = ByteArray(32).also { pack(it, n) }

private fun parity(a: LongArray): Int = encoded(a)[0].toInt() and 1

private fun decode(o: LongArray, n: ByteArray) {
    for (i in 0 until 16) o[i] = (n[2 * i].toLong() and 0xff) + ((n[2 * i + 1].toLong() and 0xff) shl 8)
    o[15] = o[15] and 0x7fff
}

private fun plus(o: LongArray, a: LongArray, b: LongArray) {
    for (i in 0 until 16) o[i] = a[i] + b[i]
}

private fun minus(o: LongArray, a: LongArray, b: LongArray) {
    for (i in 0 until 16) o[i] = a[i] - b[i]
}

private fun times(o: LongArray, a: LongArray, b: LongArray) {
    val t = LongArray(31)
    for (i in 0 until 16) for (j in 0 until 16) t[i + j] += a[i] * b[j]
    for (i in 0 until 15) t[i] += 38 * t[i + 16]
    t.copyInto(o, endIndex = 16)
    carry(o)
    carry(o)
}

/** [i] to the power 2^252 - 3, by squaring and multiplying. */
private fun pow2523(o: LongArray, i: LongArray) {
    val c = i.copyOf()
    for (a in 250 downTo 0) {
        times(c, c, c)
        if (a != 1) times(c, c, i)
    }
    c.copyInto(o)
}

/** The inverse of [i], as [i] to the power p - 2. */
private fun invert(o: LongArray, i: LongArray) {
    val c = i.copyOf()
    for (a in 253 downTo 0) {
        times(c, c, c)
        if (a != 2 && a != 4) times(c, c, i)
    }
    c.copyInto(o)
}

/** p = p + q. */
private fun add(p: Array<LongArray>, q: Array<LongArray>) {
    val a = fe()
    val b = fe()
    val c = fe()
    val d = fe()
    val t = fe()
    minus(a, p[1], p[0])
    minus(t, q[1], q[0])
    times(a, a, t)
    plus(b, p[0], p[1])
    plus(t, q[0], q[1])
    times(b, b, t)
    times(c, p[3], q[3])
    times(c, c, D2)
    times(d, p[2], q[2])
    plus(d, d, d)
    val e = fe().also { minus(it, b, a) }
    val f = fe().also { minus(it, d, c) }
    val g = fe().also { plus(it, d, c) }
    val h = fe().also { plus(it, b, a) }
    times(p[0], e, f)
    times(p[1], h, g)
    times(p[2], g, f)
    times(p[3], e, h)
}

/** p = s × q, s a 32-byte little-endian scalar. */
private fun scalarMult(p: Array<LongArray>, q: Array<LongArray>, s: ByteArray) {
    for (i in 255 downTo 0) {
        add(p, arrayOf(p[0].copyOf(), p[1].copyOf(), p[2].copyOf(), p[3].copyOf()))
        if ((s[i / 8].toInt() shr (i and 7)) and 1 == 1) add(p, q)
    }
}

/** s × the base point. */
internal fun scalarBase(s: ByteArray): Array<LongArray> {
    val p = point()
    scalarMult(p, arrayOf(BX.copyOf(), BY.copyOf(), fe(1), fe().also { times(it, BX, BY) }), s)
    return p
}

/** [p] encoded as 32 bytes: its y, with x's parity in the top bit. */
internal fun encode(p: Array<LongArray>): ByteArray {
    val zi = fe()
    val tx = fe()
    val ty = fe()
    invert(zi, p[2])
    times(tx, p[0], zi)
    times(ty, p[1], zi)
    val r = encoded(ty)
    r[31] = (r[31].toInt() xor (parity(tx) shl 7)).toByte()
    return r
}

/** r = -(the point [p] encodes); false when [p] encodes none. */
private fun unpackNegative(r: Array<LongArray>, p: ByteArray): Boolean {
    val t = fe()
    val chk = fe()
    val num = fe()
    val den = fe()
    val den2 = fe()
    val den4 = fe()
    val den6 = fe()
    decode(r[1], p)
    times(num, r[1], r[1])
    times(den, num, D)
    minus(num, num, r[2])
    plus(den, r[2], den)
    times(den2, den, den)
    times(den4, den2, den2)
    times(den6, den4, den2)
    times(t, den6, num)
    times(t, t, den)
    pow2523(t, t)
    times(t, t, num)
    times(t, t, den)
    times(t, t, den)
    times(r[0], t, den)
    times(chk, r[0], r[0])
    times(chk, chk, den)
    if (!encoded(chk).contentEquals(encoded(num))) times(r[0], r[0], SQRT_M1)
    times(chk, r[0], r[0])
    times(chk, chk, den)
    if (!encoded(chk).contentEquals(encoded(num))) return false
    if (parity(r[0]) == (p[31].toInt() and 0xff) shr 7) minus(r[0], fe(), r[0])
    times(r[3], r[0], r[1])
    return true
}

/** r = x mod L, x 64 signed limbs of 8 bits, r its 32 bytes. */
internal fun modL(r: ByteArray, x: LongArray) {
    for (i in 63 downTo 32) {
        var carry = 0L
        var j = i - 32
        while (j < i - 12) {
            x[j] += carry - 16 * x[i] * L[j - (i - 32)]
            carry = (x[j] + 128) shr 8
            x[j] -= carry shl 8
            j++
        }
        x[j] += carry
        x[i] = 0
    }
    var carry = 0L
    for (j in 0 until 32) {
        x[j] += carry - (x[31] shr 4) * L[j]
        carry = x[j] shr 8
        x[j] = x[j] and 255
    }
    for (j in 0 until 32) x[j] -= carry * L[j]
    for (i in 0 until 32) {
        x[i + 1] += x[i] shr 8
        r[i] = x[i].toByte()
    }
}

/** A 64-byte hash, as a scalar mod L in its first 32 bytes. */
internal fun reduce(h: ByteArray) {
    val x = LongArray(64) { h[it].toLong() and 0xff }
    h.fill(0)
    modL(h, x)
}
