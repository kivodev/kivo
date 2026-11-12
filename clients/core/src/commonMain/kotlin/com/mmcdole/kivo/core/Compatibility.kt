package com.mmcdole.kivo.core

/**
 * The oldest server release the apps built on this core work with. A
 * server older than it is one the app shows as needing an update, rather
 * than call it. Raise it in the change that first needs a newer server, to
 * the release that server ships in.
 */
const val MIN_SERVER_VERSION = "0.3.0"

/**
 * Whether an app and a server work together, from what each says of
 * itself: the server its version and the oldest app it supports, the app
 * its version and the oldest server it works with.
 */
sealed interface Compatibility {
    /** They work together, or one of them doesn't say enough to tell. */
    data object Compatible : Compatibility

    /** The server, at [serverVersion], is older than [minServerVersion], the oldest the app at [appVersion] works with. */
    data class ServerTooOld(val serverVersion: String, val minServerVersion: String, val appVersion: String) : Compatibility

    /** The app, at [appVersion], is older than [minAppVersion], the oldest the server supports. */
    data class AppTooOld(val appVersion: String, val minAppVersion: String) : Compatibility

    companion object {
        /**
         * How the app at [appVersion] and a server at [serverVersion] that
         * supports apps from [minAppVersion] stand. A version that isn't a
         * release's ([compareVersions]), or a server that names no oldest
         * app, never makes them incompatible.
         */
        fun of(serverVersion: String, minAppVersion: String?, appVersion: String): Compatibility {
            if ((compareVersions(serverVersion, MIN_SERVER_VERSION) ?: 0) < 0) {
                return ServerTooOld(release(serverVersion), MIN_SERVER_VERSION, release(appVersion))
            }
            if (minAppVersion != null && (compareVersions(appVersion, minAppVersion) ?: 0) < 0) {
                return AppTooOld(release(appVersion), release(minAppVersion))
            }
            return Compatible
        }
    }
}

/**
 * Compares two versions by major, minor and patch: negative when [a] is
 * older than [b], zero when they're the same release, positive when newer.
 * A leading `v` and anything from the first `-` or `+` on, such as
 * `-dev+abc1234` or an app's build number in `+12`, are ignored, so a dev
 * build or a prerelease is never older than the release it names. Null
 * when either lacks the three numbers, as `dev`, a commit hash or an empty
 * version do.
 */
internal fun compareVersions(a: String, b: String): Int? {
    val x = parse(a) ?: return null
    val y = parse(b) ?: return null
    for (i in x.indices) {
        if (x[i] != y[i]) return x[i].compareTo(y[i])
    }
    return 0
}

/** A version's major, minor and patch, as in `v0.4.0`, `0.3.1-dev+56c0018d` or `0.3.0+78`; null without all three. */
private fun parse(version: String): List<Int>? {
    val parts = version.trim().removePrefix("v").takeWhile { it != '-' && it != '+' }.split('.')
    if (parts.size != 3) return null
    return parts.map { part -> part.takeIf { it.isNotEmpty() && it.all { c -> c in '0'..'9' } }?.toIntOrNull() ?: return null }
}

/** [version] as a person reads it: its major.minor.patch; as it is when it has none. */
private fun release(version: String): String = parse(version)?.joinToString(".") ?: version
