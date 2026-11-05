package com.mmcdole.kivo.core.auth

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import com.mmcdole.kivo.api.models.ServerInfo
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import kotlinx.serialization.Serializable
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.Json

/** The servers this app knows, kept in its DataStore. */
internal class ServerList(private val store: DataStore<Preferences>) {
    private val key = stringPreferencesKey("kivo.servers")
    private val serializer = ListSerializer(SavedServer.serializer())

    val servers: Flow<List<SavedServer>> = store.data.map { prefs -> decode(prefs) }

    /** Adds [server], or replaces the one with its ID. */
    suspend fun save(server: SavedServer) {
        store.edit { prefs -> prefs[key] = Json.encodeToString(serializer, decode(prefs).filter { it.id != server.id } + server) }
    }

    suspend fun remove(id: String) {
        store.edit { prefs -> prefs[key] = Json.encodeToString(serializer, decode(prefs).filter { it.id != id }) }
    }

    private fun decode(prefs: Preferences): List<SavedServer> =
        prefs[key]?.let { Json.decodeFromString(serializer, it) }.orEmpty()
}

/**
 * A server this app has signed in to, keyed by its stable ID. [addresses] are
 * tried in order: its local URLs first, then the address it was reached at,
 * then its public URL, so a phone set up at home finds it at home and away.
 */
@Serializable
data class SavedServer(val id: String, val name: String, val addresses: List<String>) {
    /** The server as [info] describes it, reached at [address]; an IPv6 link-local address isn't kept. */
    constructor(info: ServerInfo, address: String) : this(
        id = info.id,
        name = info.name,
        addresses = (info.localUrls.orEmpty() + address + listOfNotNull(info.publicUrl)).filterNot(::isLinkLocal).distinct(),
    )
}
