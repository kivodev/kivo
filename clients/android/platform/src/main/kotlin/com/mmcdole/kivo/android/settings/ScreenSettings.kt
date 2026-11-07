package com.mmcdole.kivo.android.settings

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

/** How text subtitles the player draws look; styled ones keep their own. */
enum class SubtitleSize { Small, Medium, Large }

enum class SubtitleBackground { None, Shadow, Box }

/**
 * This TV's settings that only the app's own screens follow, as the player
 * draws subtitles and sets up the display; the core keeps the rest.
 */
data class ScreenSettings(
    val subtitleSize: SubtitleSize = SubtitleSize.Medium,
    val subtitleBackground: SubtitleBackground = SubtitleBackground.Shadow,
    /** Switching the display to the video's frame rate, below Android 12; the system's setting decides from 12. */
    val frameRateMatching: Boolean = true,
)

/** [ScreenSettings] in the app's DataStore, defaults for anything unset or unknown. */
class ScreenSettingsStore(private val store: DataStore<Preferences>) {
    val values: Flow<ScreenSettings> = store.data.map { prefs ->
        ScreenSettings(
            subtitleSize = SubtitleSize.entries.firstOrNull { it.name == prefs[SIZE] } ?: SubtitleSize.Medium,
            subtitleBackground = SubtitleBackground.entries.firstOrNull { it.name == prefs[BACKGROUND] } ?: SubtitleBackground.Shadow,
            frameRateMatching = prefs[FRAME_RATE] ?: true,
        )
    }

    suspend fun setSubtitleSize(size: SubtitleSize) {
        store.edit { it[SIZE] = size.name }
    }

    suspend fun setSubtitleBackground(background: SubtitleBackground) {
        store.edit { it[BACKGROUND] = background.name }
    }

    suspend fun setFrameRateMatching(on: Boolean) {
        store.edit { it[FRAME_RATE] = on }
    }

    private companion object {
        val SIZE = stringPreferencesKey("kivo.subtitleSize")
        val BACKGROUND = stringPreferencesKey("kivo.subtitleBackground")
        val FRAME_RATE = booleanPreferencesKey("kivo.frameRateMatching")
    }
}
