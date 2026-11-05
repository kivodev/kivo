package com.mmcdole.kivo.core.auth

/**
 * Secrets kept on the device, implemented by each platform: the Keystore on
 * Android (`KeystoreSecureStore`), the Keychain on Apple. Each server's
 * session token is kept here under the server's ID.
 */
interface SecureStore {
    suspend fun read(key: String): String?

    suspend fun write(key: String, value: String)

    suspend fun delete(key: String)
}
