package com.mmcdole.kivo.core.auth

import java.security.MessageDigest
import java.security.SecureRandom

internal actual fun sha512(data: ByteArray): ByteArray = MessageDigest.getInstance("SHA-512").digest(data)

private val random = SecureRandom()

internal actual fun secureRandom(size: Int): ByteArray = ByteArray(size).also(random::nextBytes)
