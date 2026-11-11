package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.api.models.ServerProof
import kotlin.test.Test
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

// A proof the Go server signed (auth.SignProbe), checked by each platform's
// own crypto.

private const val KEY = "BI2ne22wXIhyKuKcWEggOjQHviITw-otwB_GzsSPja5qNN3YgVoz56b9LLk92NZTtdIleYsrl0TvieDiBvcvNnA"
private const val SIGNATURE = "MEYCIQDtJdxnqhGF_ExiqHNiqC9kaxLHgAdpgKupqoaKwvHVOwIhAM1ai0SXWvqWjqj70VKz_xvwbByB3X2-25egh3S_JsLD"
private const val SERVER_ID = "01a1cc3a-79f2-7354-a1da-2febd591d90f"
private val CHALLENGE = "0123456789abcdef0123456789abcdef".encodeToByteArray()

/** Another P-256 key, from the same server code. */
private const val OTHER_KEY = "BLdcS5VyPqCqXLRQ5-JhZ7Ot3LrWmNLh2ROW4HTZXi7h4glC2D9bO2rijRrE4iyBVcBuImZW0q-5lr2ABZu6NMQ"

class ServerKeyTest {
    private val key = assertNotNull(ServerKey.of(KEY))
    private val proof = ServerProof(SIGNATURE, KEY, "192.168.1.20:8484", local = true)

    @Test
    fun theServersSignatureVerifies() {
        assertTrue(key.signed(proof, CHALLENGE, SERVER_ID))
    }

    @Test
    fun anythingChangedDoesnt() {
        assertFalse(key.signed(proof, "another challenge, of 32 bytes!!".encodeToByteArray(), SERVER_ID), "challenge")
        assertFalse(key.signed(proof, CHALLENGE, "another-server"), "server")
        assertFalse(key.signed(proof.copy(host = "192.168.1.21:8484"), CHALLENGE, SERVER_ID), "host")
        assertFalse(key.signed(proof.copy(local = false), CHALLENGE, SERVER_ID), "local")
        val flipped = SIGNATURE.substring(0, 20) + (if (SIGNATURE[20] == 'A') 'B' else 'A') + SIGNATURE.substring(21)
        assertFalse(key.signed(proof.copy(signature = flipped), CHALLENGE, SERVER_ID), "signature")
        assertFalse(key.signed(proof.copy(signature = "not a signature"), CHALLENGE, SERVER_ID), "garbage")
        assertFalse(key.signed(null, CHALLENGE, SERVER_ID), "no proof")
        assertFalse(assertNotNull(ServerKey.of(OTHER_KEY)).signed(proof, CHALLENGE, SERVER_ID), "another key")
    }

    @Test
    fun onlyAnUncompressedPointIsAKey() {
        assertNull(ServerKey.of(null))
        assertNull(ServerKey.of("AAAA"))
        assertNull(ServerKey.of("A" + KEY.substring(1)), "not uncompressed")
    }
}
