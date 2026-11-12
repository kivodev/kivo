package com.mmcdole.kivo.android

import android.content.res.Resources
import com.mmcdole.kivo.android.platform.R
import com.mmcdole.kivo.core.Compatibility

/** What the screen for a server the app doesn't work with says: its [title] and a [sentence] under it. */
data class IncompatibleWords(val title: String, val sentence: String)

/** What to say of [server] when the app and it don't work together; null when they do. */
fun Compatibility.words(resources: Resources, server: String): IncompatibleWords? = when (this) {
    Compatibility.Compatible -> null
    is Compatibility.ServerTooOld -> IncompatibleWords(
        resources.getString(R.string.server_too_old_title),
        resources.getString(R.string.server_too_old_sentence, appVersion, minServerVersion, server, serverVersion),
    )
    is Compatibility.AppTooOld -> IncompatibleWords(
        resources.getString(R.string.app_too_old_title),
        resources.getString(R.string.app_too_old_sentence, server, minAppVersion, appVersion),
    )
}
