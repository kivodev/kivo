package com.mmcdole.kivo.core.settings

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.MutablePreferences
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.core.stringSetPreferencesKey
import com.mmcdole.kivo.api.models.ClientPlatform
import com.mmcdole.kivo.api.models.QualityRung
import com.mmcdole.kivo.core.caps.AudioSettings
import com.mmcdole.kivo.core.caps.PassthroughMode
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

/**
 * This device's settings the core plays and downloads by ("This device" in
 * Settings), kept on the device: a phone and a TV signed in as one user want
 * different ones. Those only an app's own screens follow, such as its theme
 * and its subtitle style, the app keeps itself.
 */
data class DeviceSettings(
    /** For a play on the server's local networks. */
    val localQuality: Quality = Quality.Auto,
    /** For a play from anywhere else. */
    val remoteQuality: Quality = Quality.Auto,
    /** Passthrough and Maximum audio channels; passthrough on Android TV only. */
    val audio: AudioSettings = AudioSettings(),
    /** Whether plays boost dialogue, as switched in the player. */
    val dialogueBoost: Boolean = false,
    /** The quality videos download at, unless a download asks for another (phones and tablets): Standard on a phone, else High. */
    val downloadQuality: DownloadQuality = DownloadQuality.High,
    /** Whether downloads may use mobile data; otherwise they wait for Wi-Fi. */
    val cellularDownloads: Boolean = false,
) {
    /** These settings with both qualities as the server offers them ([Quality.onLadder]). */
    fun onLadder(rungs: List<QualityRung>): DeviceSettings =
        copy(localQuality = localQuality.onLadder(rungs), remoteQuality = remoteQuality.onLadder(rungs))
}

/**
 * A download's quality, by the name the server knows it by: the tallest
 * picture and the cap on the video's bitrate a conversion makes. High is
 * for any screen, Standard for a phone's. The original goes as it is when
 * it's no larger than the quality's conversion could be.
 */
enum class DownloadQuality(val value: String, val height: Int?, val bitrateBps: Long?) {
    Original("original", null, null),
    High("high", 1080, 8_000_000),
    Standard("standard", 1080, 3_000_000),
    ;

    companion object {
        /** The quality by its name, or null for one this version doesn't know. */
        fun parseOrNull(value: String?): DownloadQuality? {
            // Migration (downloads): remove after every client has updated past this release.
            val name = value?.let(LegacyDownloadQuality::current)
            return entries.firstOrNull { it.value == name }
        }

        /** The quality a device on [platform] downloads at until it's set: Standard on a phone, else High. */
        internal fun defaultOn(platform: ClientPlatform): DownloadQuality =
            if (platform == ClientPlatform.IPhone || platform == ClientPlatform.AndroidPhone) Standard else High
    }
}

// Migration (downloads): remove after every client has updated past this release.
/** The download qualities an earlier version offered, each by the quality it is now. */
internal object LegacyDownloadQuality {
    // Migration (downloads): remove after every client has updated past this release.
    private val qualities = mapOf("1080p-high" to "high", "1080p" to "high", "720p" to "standard", "480p" to "standard")

    // Migration (downloads): remove after every client has updated past this release.
    /** A quality's name as this version knows it: 1080p-high and 1080p are High, 720p and 480p Standard. */
    fun current(name: String): String = qualities[name] ?: name
}


/**
 * This device's settings in the app's DataStore, defaults for anything
 * unset or unknown, the download quality's [downloadDefault].
 */
internal class DeviceSettingsStore(
    private val store: DataStore<Preferences>,
    private val downloadDefault: DownloadQuality = DownloadQuality.High,
) {
    val values: Flow<DeviceSettings> = store.data.map { it.read() }

    /** Saves [transform] of the current values. */
    suspend fun update(transform: (DeviceSettings) -> DeviceSettings) {
        store.edit { prefs -> prefs.write(transform(prefs.read())) }
    }

    private fun Preferences.read() = DeviceSettings(
        localQuality = Quality.of(this[Keys.localQuality] ?: Legacy.localQuality(this)),
        remoteQuality = Quality.of(this[Keys.remoteQuality]),
        audio = AudioSettings(
            // An earlier version's On reads as Auto.
            passthrough = enumOf(this[Keys.passthrough], PassthroughMode.Auto),
            maxChannels = this[Keys.maxAudioChannels]?.takeIf { it in listOf(2, 6, 8) },
        ),
        dialogueBoost = this[Keys.dialogueBoost] ?: false,
        downloadQuality = DownloadQuality.parseOrNull(this[Keys.downloadQuality]) ?: downloadDefault,
        cellularDownloads = this[Keys.cellularDownloads] ?: false,
    )

    private fun MutablePreferences.write(values: DeviceSettings) {
        set(Keys.localQuality, values.localQuality.key)
        set(Keys.remoteQuality, values.remoteQuality.key)
        set(Keys.passthrough, values.audio.passthrough.name)
        values.audio.maxChannels?.let { set(Keys.maxAudioChannels, it) } ?: remove(Keys.maxAudioChannels)
        set(Keys.dialogueBoost, values.dialogueBoost)
        set(Keys.downloadQuality, values.downloadQuality.value)
        set(Keys.cellularDownloads, values.cellularDownloads)
        remove(Keys.removeWhenFinished)
        remove(Keys.passthroughOff)
        Legacy.remove(this)
    }

    private object Keys {
        val localQuality = stringPreferencesKey("kivo.quality.local")
        val remoteQuality = stringPreferencesKey("kivo.quality.remote")
        val passthrough = stringPreferencesKey("kivo.audio.passthrough")
        val maxAudioChannels = intPreferencesKey("kivo.audio.maxChannels")
        val dialogueBoost = booleanPreferencesKey("kivo.audio.dialogueBoost")
        val downloadQuality = stringPreferencesKey("kivo.download.quality")
        val cellularDownloads = booleanPreferencesKey("kivo.download.cellular")
        // Settings no longer kept: an earlier version's value goes at the next save.
        val removeWhenFinished = booleanPreferencesKey("kivo.download.removeWhenFinished")
        val passthroughOff = stringSetPreferencesKey("kivo.audio.passthroughOff")
    }

    // Migration (api-naming): remove after every client has updated past this release.
    /** The local quality as an earlier version stored it, read until the next save writes it anew. */
    private object Legacy {
        // Migration (api-naming): remove after every client has updated past this release.
        private val localQuality = stringPreferencesKey("kivo.quality.home")

        // Migration (api-naming): remove after every client has updated past this release.
        fun localQuality(prefs: Preferences): String? = prefs[localQuality]

        // Migration (api-naming): remove after every client has updated past this release.
        fun remove(prefs: MutablePreferences) {
            prefs.remove(localQuality)
        }
    }
}

private inline fun <reified T : Enum<T>> enumOf(name: String?, default: T): T =
    enumValues<T>().firstOrNull { it.name == name } ?: default
