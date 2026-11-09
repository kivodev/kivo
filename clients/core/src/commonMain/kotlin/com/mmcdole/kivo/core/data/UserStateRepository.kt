package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.UserState
import com.mmcdole.kivo.api.check
import com.mmcdole.kivo.api.models.UserStateUpdate
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection

/** The user's played marks; the live events tell every screen, this app's too. */
internal class UserStateRepository(private val connection: ServerConnection) {
    suspend fun setPlayed(workId: String, played: Boolean): UserState =
        connection.works.updateWorkUserState(workId, UserStateUpdate(played = played)).result()

    suspend fun removeFromContinue(workId: String) = connection.works.removeFromContinue(workId).check()
}
