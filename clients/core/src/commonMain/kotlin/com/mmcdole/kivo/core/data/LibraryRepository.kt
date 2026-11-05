package com.mmcdole.kivo.core.data

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import com.mmcdole.kivo.api.models.Library
import com.mmcdole.kivo.api.result
import com.mmcdole.kivo.core.auth.ServerConnection
import com.mmcdole.kivo.core.kivoJson
import com.mmcdole.kivo.core.live.Invalidation
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.onEach
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException

/**
 * The libraries the user sees, in the user's order. Each answer is kept on
 * the device, in the app's [preferences] under the server's ID with the
 * user it's for, so the libraries show before the server answers.
 */
internal class LibraryRepository(
    private val connection: ServerConnection,
    private val invalidations: Flow<Invalidation>,
    private val preferences: DataStore<Preferences>,
) {
    private val key = stringPreferencesKey("kivo.libraries.${connection.serverId}")

    fun libraries(): Flow<List<Library>> = refreshing(invalidations, { it is Invalidation.LibraryChanged }) {
        connection.libraries.listLibraries().result().items
    }.onEach(::keep)

    /** The libraries as last read for the user signed in now; null when none are kept. */
    suspend fun kept(): List<Library>? {
        val user = connection.user.value?.id ?: return null
        val stored = preferences.data.first()[key] ?: return null
        return try {
            kivoJson.decodeFromString(KeptLibraries.serializer(), stored).takeIf { it.userId == user }?.libraries
        } catch (_: SerializationException) {
            null
        }
    }

    private suspend fun keep(libraries: List<Library>) {
        val user = connection.user.value?.id ?: return
        try {
            preferences.edit { it[key] = kivoJson.encodeToString(KeptLibraries.serializer(), KeptLibraries(user, libraries)) }
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            // The answer still shows; it just isn't kept.
        }
    }
}

@Serializable
private class KeptLibraries(val userId: String, val libraries: List<Library>)
