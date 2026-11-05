package com.mmcdole.kivo.core.auth

/**
 * What the app says of itself to the servers it signs in to, in the
 * identity headers of every request the core makes. The server keeps it
 * with the session and its plays, for "Now Playing" and the signed-in
 * devices. Implemented by each app; never the screen size or the user's
 * name for the device.
 */
interface ClientIdentity {
    /** `kivo-android` or `kivo-apple`. */
    val app: String

    /** The app's version, such as `1.2.3`. */
    val appVersion: String

    /** What it runs on: `androidTv`, `androidPhone`, `appleTv`, `iPhone` or `iPad`. */
    val platform: String

    /** The operating system's version, such as `14` or `17.5`. */
    val osVersion: String

    /** The device's model, coarse: Android's `Build.MODEL`, or Apple's machine identifier, such as `iPhone16,2`. */
    val deviceModel: String
}

/**
 * The identity headers for [identity], a header per field. Each value
 * keeps printable ASCII only and at most [MAX_VALUE] characters; one left
 * empty is left out.
 */
internal fun identityHeaders(identity: ClientIdentity): Map<String, String> = listOf(
    "Kivo-App" to identity.app,
    "Kivo-App-Version" to identity.appVersion,
    "Kivo-Platform" to identity.platform,
    "Kivo-OS-Version" to identity.osVersion,
    "Kivo-Device-Model" to identity.deviceModel,
).mapNotNull { (name, value) ->
    value.filter { it in ' '..'~' }.trim().take(MAX_VALUE).takeIf { it.isNotEmpty() }?.let { name to it }
}.toMap()

/** The most characters a header value keeps. */
private const val MAX_VALUE = 100
