package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.api.models.CurrentUser
import kotlinx.serialization.Serializable

/**
 * Who this app is signed in as, kept on the device beside the session's
 * token so it shows without the server. The server stays the authority on
 * what the user may do; [admin] only picks what the app offers.
 */
@Serializable
data class SignedInUser(val id: String, val username: String, val admin: Boolean) {
    /** The first letter of [username], upper-cased, for an avatar. */
    val initial: String
        get() = username.take(if (username.firstOrNull()?.isHighSurrogate() == true) 2 else 1).uppercase()

    internal constructor(user: CurrentUser) : this(user.id, user.username, user.admin)
}
