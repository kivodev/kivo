package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.api.models.ServerProof
import io.ktor.http.Url
import kotlin.io.encoding.Base64

/**
 * What a probe of a plain-HTTP address needs to tell the server this app
 * signed in to from an impostor, from the session's [token]: the key the
 * server holds too, SHA-256 of the token (the hash it keeps), and the
 * session's [probeId], which the probe sends in the clear. The server
 * proves itself with HMAC-SHA256, keyed by that hash, over the probe's
 * fresh challenge, its ID, the host the probe reached and whether the probe
 * came from a local network ([proves]). Null for a token that isn't one.
 */
internal class ProbeKey private constructor(private val key: ByteArray) {
    /** The first 16 bytes of SHA-256("kivo-probe-id" ‖ key), base64url. */
    val probeId: String = base64url.encode(sha256("kivo-probe-id".encodeToByteArray() + key).copyOf(PROBE_ID_BYTES))

    /** A new challenge for one probe. */
    fun challenge(): ByteArray = secureRandom(CHALLENGE_BYTES)

    /**
     * Whether [proof] proves that [address], probed with [challenge], is
     * server [serverId], reached on a local network at the very host probed:
     * its MAC is right, it says the probe came from a local network, and it
     * names the host of [address]. A missing or wrong proof proves nothing.
     */
    fun proves(proof: ServerProof?, challenge: ByteArray, serverId: String, address: String): Boolean {
        if (proof == null) return false
        val mac = try {
            base64url.decode(proof.mac)
        } catch (_: IllegalArgumentException) {
            return false
        }
        val want = hmacSha256(key, message(challenge, serverId, proof.host, proof.local))
        return constantTimeEquals(mac, want) && proof.local && sameHost(proof.host, address)
    }

    companion object {
        /** The key and probe ID of [token]; null when it isn't a token. */
        fun of(token: String): ProbeKey? {
            val raw = try {
                base64url.decode(token)
            } catch (_: IllegalArgumentException) {
                return null
            }
            return ProbeKey(sha256(raw))
        }

        /** The base64url of [bytes], without padding, as a probe sends its challenge. */
        fun encode(bytes: ByteArray): String = base64url.encode(bytes)
    }
}

private val base64url = Base64.UrlSafe.withPadding(Base64.PaddingOption.ABSENT)

private const val PROBE_ID_BYTES = 16
private const val CHALLENGE_BYTES = 32

/**
 * What the server's MAC covers: "kivo-probe-v1", then the challenge, the
 * server's ID and the host, each after its length as two bytes big-endian,
 * then one byte, 1 when local.
 */
internal fun message(challenge: ByteArray, serverId: String, host: String, local: Boolean): ByteArray {
    val out = ArrayList<Byte>()
    out += "kivo-probe-v1".encodeToByteArray().toList()
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
