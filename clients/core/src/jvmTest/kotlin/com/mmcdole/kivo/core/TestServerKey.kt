package com.mmcdole.kivo.core

import com.mmcdole.kivo.api.models.ServerProof
import com.mmcdole.kivo.core.auth.message
import io.ktor.client.request.HttpRequestData
import java.security.KeyPairGenerator
import java.security.Signature
import java.security.spec.ECGenParameterSpec
import java.util.Base64

private val base64url = Base64.getUrlEncoder().withoutPadding()

/** A server's ECDSA P-256 identity key, made anew, which signs as the server does: SHA-256, ASN.1 DER. */
internal class TestServerKey {
    private val pair = KeyPairGenerator.getInstance("EC").apply { initialize(ECGenParameterSpec("secp256r1")) }.generateKeyPair()

    /** The public key as the server gives it: the uncompressed point, the end of its X.509 encoding. */
    val encoded: String = base64url.encodeToString(pair.public.encoded.takeLast(65).toByteArray())

    fun sign(message: ByteArray): ByteArray = Signature.getInstance("SHA256withECDSA").run {
        initSign(pair.private)
        update(message)
        sign()
    }
}

/** The test server's identity key. */
internal val SERVER_KEY = TestServerKey()

/**
 * The proof a server with [key] answers a probe [request] with, as reached
 * at the probed host from a local network; null for a request that isn't a
 * probe.
 */
internal fun proofOf(request: HttpRequestData, serverId: String, key: TestServerKey = SERVER_KEY): ServerProof? =
    proofOf(request.url.parameters["challenge"], "${request.url.host}:${request.url.port}", serverId, key)

/** The proof to a probe with [challenge] that reached [host]; null without a challenge. */
internal fun proofOf(challenge: String?, host: String, serverId: String, key: TestServerKey = SERVER_KEY): ServerProof? {
    if (challenge == null) return null
    val signature = key.sign(message(Base64.getUrlDecoder().decode(challenge), serverId, host, local = true))
    return ServerProof(base64url.encodeToString(signature), key.encoded, host, local = true)
}
