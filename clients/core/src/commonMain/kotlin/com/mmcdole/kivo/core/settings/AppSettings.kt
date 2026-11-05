package com.mmcdole.kivo.core.settings

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey

/**
 * The settings an earlier version kept in [store] for an app's own screens,
 * which the app keeps itself now: `theme` (`Dark` or `Light`),
 * `subtitleSize` (`Small`, `Medium` or `Large`), `subtitleBackground`
 * (`None`, `Shadow` or `Box`) and `keepScreenOn` (`true` or `false`), those
 * set. Each is removed as it's taken, so they're taken once.
 */
internal suspend fun takeEarlierAppSettings(store: DataStore<Preferences>): Map<String, String> {
    var taken = emptyMap<String, String>()
    store.edit { prefs ->
        taken = buildMap {
            for ((name, key) in TEXT) prefs[key]?.let { put(name, it) }
            prefs[KEEP_SCREEN_ON]?.let { put("keepScreenOn", it.toString()) }
        }
        TEXT.forEach { (_, key) -> prefs.remove(key) }
        prefs.remove(KEEP_SCREEN_ON)
    }
    return taken
}

private val TEXT = listOf(
    "theme" to stringPreferencesKey("kivo.theme"),
    "subtitleSize" to stringPreferencesKey("kivo.subtitleSize"),
    "subtitleBackground" to stringPreferencesKey("kivo.subtitleBackground"),
)

private val KEEP_SCREEN_ON = booleanPreferencesKey("kivo.download.keepScreenOn")
