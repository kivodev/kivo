package com.mmcdole.kivo.core.auth

import androidx.datastore.core.DataMigration
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import kotlinx.coroutines.flow.first
import kotlin.uuid.Uuid

private val CLIENT_ID = stringPreferencesKey("kivo.clientId")

/**
 * This install's client ID, which [SignIn] sends: random, made the first
 * time it's asked for and kept in the app's DataStore, so signing in again
 * from this install replaces its session on the server rather than adding
 * one.
 */
internal class ClientId(private val store: DataStore<Preferences>) {
    suspend fun get(): String {
        store.data.first()[CLIENT_ID]?.let { return it }
        val prefs = store.edit { if (it[CLIENT_ID] == null) it[CLIENT_ID] = Uuid.random().toString() }
        return checkNotNull(prefs[CLIENT_ID])
    }
}

/**
 * Makes [id] this install's client ID as the app's DataStore opens, as UI
 * tests fix one so every run keeps to one session.
 */
internal fun fixedClientId(id: String): DataMigration<Preferences> = object : DataMigration<Preferences> {
    override suspend fun shouldMigrate(currentData: Preferences): Boolean = currentData[CLIENT_ID] != id

    override suspend fun migrate(currentData: Preferences): Preferences =
        currentData.toMutablePreferences().apply { this[CLIENT_ID] = id }

    override suspend fun cleanUp() = Unit
}
