package com.mmcdole.kivo.core.settings

import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.core.stringSetPreferencesKey
import com.mmcdole.kivo.api.models.ClientPlatform
import com.mmcdole.kivo.api.models.QualityRequest
import com.mmcdole.kivo.core.caps.AudioSettings
import com.mmcdole.kivo.core.caps.PassthroughMode
import com.mmcdole.kivo.core.tempDataStore
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class DeviceSettingsStoreTest {
    @Test
    fun startsAtTheDefaultsAndKeepsChanges() = runTest {
        val settings = DeviceSettingsStore(backgroundScope.tempDataStore())
        assertEquals(DeviceSettings(), settings.values.first())

        val changed = DeviceSettings(
            localQuality = Quality.Original,
            remoteQuality = Quality.Limit(4_000_000),
            audio = AudioSettings(PassthroughMode.Off, maxChannels = 6),
            dialogueBoost = true,
            downloadQuality = DownloadQuality.Standard,
        )
        settings.update { changed }
        assertEquals(changed, settings.values.first())

        settings.update { it.copy(audio = it.audio.copy(maxChannels = null)) }
        assertEquals(null, settings.values.first().audio.maxChannels)
    }

    // Migration (api-naming): remove after every client has updated past this release.
    @Test
    fun anEarlierVersionsHomeQualityReadsAsTheLocalQualityUntilTheNextSave() = runTest {
        val store = backgroundScope.tempDataStore()
        val settings = DeviceSettingsStore(store)
        val earlier = stringPreferencesKey("kivo.quality.home")
        store.edit { it[earlier] = "8000000" }
        assertEquals(Quality.Limit(8_000_000), settings.values.first().localQuality)
        settings.update { it.copy(dialogueBoost = true) }
        assertNull(store.data.first()[earlier])
        assertEquals("8000000", store.data.first()[stringPreferencesKey("kivo.quality.local")])
    }

    @Test
    fun aStoredDownloadQualityKeepsItsName() = runTest {
        val store = backgroundScope.tempDataStore()
        val settings = DeviceSettingsStore(store)
        val key = stringPreferencesKey("kivo.download.quality")
        for ((stored, quality) in listOf(
            "high" to DownloadQuality.High, "standard" to DownloadQuality.Standard,
            "original" to DownloadQuality.Original, "4k" to DownloadQuality.High,
        )) {
            store.edit { it[key] = stored }
            assertEquals(quality, settings.values.first().downloadQuality, stored)
        }
        assertEquals(8_000_000, DownloadQuality.High.bitrateBps)
        assertEquals(3_000_000, DownloadQuality.Standard.bitrateBps)
    }

    @Test
    fun aPhoneDownloadsAtStandardUntilSetAndAnythingElseAtHigh() = runTest {
        val phone = DeviceSettingsStore(backgroundScope.tempDataStore(), DownloadQuality.defaultOn(ClientPlatform.IPhone))
        assertEquals(DownloadQuality.Standard, phone.values.first().downloadQuality)
        phone.update { it.copy(downloadQuality = DownloadQuality.Original) }
        assertEquals(DownloadQuality.Original, phone.values.first().downloadQuality)
        for (platform in listOf("iPad", "androidTv", "appleTv")) assertEquals(DownloadQuality.High, DownloadQuality.defaultOn(platform), platform)
        assertEquals(DownloadQuality.Standard, DownloadQuality.defaultOn(ClientPlatform.AndroidPhone))
    }

    // Migration (downloads): remove after every client has updated past this release.
    @Test
    fun anEarlierVersionsDownloadQualityReadsAsItsQualityNowAndIsSavedSo() = runTest {
        val store = backgroundScope.tempDataStore()
        val settings = DeviceSettingsStore(store, DownloadQuality.Standard)
        val key = stringPreferencesKey("kivo.download.quality")
        for ((stored, quality) in listOf(
            "1080p-high" to DownloadQuality.High, "1080p" to DownloadQuality.High,
            "720p" to DownloadQuality.Standard, "480p" to DownloadQuality.Standard,
        )) {
            store.edit { it[key] = stored }
            assertEquals(quality, settings.values.first().downloadQuality, stored)
        }
        store.edit { it[key] = "1080p" }
        settings.update { it.copy(dialogueBoost = true) }
        assertEquals("high", store.data.first()[key])
    }

    @Test
    fun anEarlierVersionsRemoveWhenFinishedIsIgnoredAndGoesAtTheNextSave() = runTest {
        val store = backgroundScope.tempDataStore()
        val settings = DeviceSettingsStore(store)
        val retired = booleanPreferencesKey("kivo.download.removeWhenFinished")
        store.edit { it[retired] = false }
        assertEquals(DeviceSettings(), settings.values.first())

        settings.update { it.copy(cellularDownloads = true) }
        assertEquals(DeviceSettings(cellularDownloads = true), settings.values.first())
        assertNull(store.data.first()[retired])
    }

    @Test
    fun anEarlierVersionsPassthroughOnIsAutoAndItsFormatsGoAtTheNextSave() = runTest {
        val store = backgroundScope.tempDataStore()
        val settings = DeviceSettingsStore(store)
        val formatsOff = stringSetPreferencesKey("kivo.audio.passthroughOff")
        for ((stored, mode) in listOf("On" to PassthroughMode.Auto, "Auto" to PassthroughMode.Auto, "Off" to PassthroughMode.Off)) {
            store.edit {
                it[stringPreferencesKey("kivo.audio.passthrough")] = stored
                it[formatsOff] = setOf("TrueHd")
            }
            assertEquals(mode, settings.values.first().audio.passthrough, stored)
        }
        settings.update { it.copy(dialogueBoost = true) }
        assertNull(store.data.first()[formatsOff])
    }

    @Test
    fun theAppsOwnSettingsAreLeftForTheAppAndTakenOnce() = runTest {
        val store = backgroundScope.tempDataStore()
        val frameRate = booleanPreferencesKey("kivo.frameRateMatching")
        store.edit {
            it[stringPreferencesKey("kivo.theme")] = "Light"
            it[stringPreferencesKey("kivo.subtitleSize")] = "Large"
            it[booleanPreferencesKey("kivo.download.keepScreenOn")] = false
            it[frameRate] = false
        }
        DeviceSettingsStore(store).update { it.copy(dialogueBoost = true) }

        assertEquals(mapOf("theme" to "Light", "subtitleSize" to "Large", "keepScreenOn" to "false"), takeEarlierAppSettings(store))
        assertEquals(emptyMap(), takeEarlierAppSettings(store))
        assertEquals(false, store.data.first()[frameRate], "the Android app's own, in the same store")
    }

    @Test
    fun qualitiesBecomeRequests() {
        assertEquals(QualityRequest("auto", bandwidthBps = 50_000_000), Quality.Auto.request(50_000_000))
        assertEquals(QualityRequest("original"), Quality.Original.request())
        assertEquals(QualityRequest("limit", maxBitrateBps = 8_000_000), Quality.Limit(8_000_000).request())
    }
}
