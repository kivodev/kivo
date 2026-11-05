package com.mmcdole.kivo.core.data

import com.mmcdole.kivo.api.models.Home
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.flow.Flow

/** Home's rows, refetched when root works, works or the user's state change. */
internal class HomeRepository(private val connection: ServerConnection, private val invalidations: Flow<Invalidation>) {
    fun home(): Flow<Home> = refreshing(invalidations, { it !is Invalidation.PlayEnded && it != Invalidation.DownloadsChanged }) {
        connection.works.getHome().result()
    }
}
