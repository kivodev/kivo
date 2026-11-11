package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.api.models.ServerProof
import io.ktor.http.Url
import kotlin.io.encoding.Base64

/**
 * The server's identity key, its ECDSA P-256 public key as the app keeps it
 * ([encoded]: the uncompressed X9.63 point, base64url): what tells the
 * server this app signed in to from
 * an impostor at a plain-HTTP address. The server signs its answer to a
 * probe's fresh challenge with it, over its ID, the host the probe reached
 * and whether the probe came from a local network ([signed]). Null for an
 * encoding that isn't one.
 */
internal class ServerKey private constructor(val encoded: String, private val key: ByteArray) {
    /** Whether [proof] is this key's signature of [challenge], [serverId] and the host and locality it names. */
    fun signed(proof: ServerProof?, challenge: ByteArray, serverId: String): Boolean {
        val signature = proof?.let { decode(it.signature) } ?: return false
        return verifyP256(key, message(challenge, serverId, proof.host, proof.local), signature)
    }

    companion object {
        fun of(encoded: String?): ServerKey? =
            encoded?.let(::decode)?.takeIf { it.size == POINT_BYTES && it[0] == UNCOMPRESSED }?.let { ServerKey(encoded, it) }

        /** A new challenge for one probe, and how the probe sends it. */
        fun challenge(): Pair<ByteArray, String> = secureRandom(CHALLENGE_BYTES).let { it to base64url.encode(it) }
    }
}

private val base64url = Base64.UrlSafe.withPadding(Base64.PaddingOption.ABSENT)

private fun decode(text: String): ByteArray? = try {
    base64url.decode(text)
} catch (_: IllegalArgumentException) {
    null
}

private const val CHALLENGE_BYTES = 32

/** An uncompressed P-256 point: its tag, then x and y. */
private const val POINT_BYTES = 65
private const val UNCOMPRESSED: Byte = 4

/**
 * What the server's signature covers: "kivo-probe-v2", then the challenge,
 * the server's ID and the host, each after its length as two bytes
 * big-endian, then one byte, 1 when local.
 */
internal fun message(challenge: ByteArray, serverId: String, host: String, local: Boolean): ByteArray {
    val out = ArrayList<Byte>()
    out += "kivo-probe-v2".encodeToByteArray().toList()
    for (field in listOf(challenge, serverId.encodeToByteArray(), host.encodeToByteArray())) {
        out += (field.size shr 8).toByte()
        out += field.size.toByte()
        out += field.toList()
    }
    out += (if (local) 1 else 0).toByte()
    return out.toByteArray()
}

/** Whether [host], as a `Host` header names it, is [address]'s host and port, its scheme's port when it names none. */
internal fun sameHost(host: String, address: String): Boolean {
    val url = try {
        Url(address)
    } catch (_: Exception) {
        return false
    }
    val (name, port) = if (host.startsWith("[")) {
        val end = host.indexOf(']').takeIf { it > 0 } ?: return false
        host.substring(1, end) to host.substring(end + 1).removePrefix(":")
    } else if (host.count { it == ':' } == 1) {
        host.substringBefore(':') to host.substringAfter(':')
    } else {
        host to ""
    }
    val wantPort = if (port.isEmpty()) url.protocol.defaultPort else port.toIntOrNull() ?: return false
    return name.equals(url.host.removePrefix("[").removeSuffix("]"), ignoreCase = true) && wantPort == url.port
}
