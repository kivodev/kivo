package com.mmcdole.kivo.core.settings

import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import com.mmcdole.kivo.api.models.ClientPlatform
import com.mmcdole.kivo.api.models.QualityRequest
import com.mmcdole.kivo.core.caps.AudioSettings
import com.mmcdole.kivo.core.caps.PassthroughMode
import com.mmcdole.kivo.core.tempDataStore
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals

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
        for (platform in listOf(ClientPlatform.IPad, ClientPlatform.AndroidTv, ClientPlatform.AppleTv)) assertEquals(DownloadQuality.High, DownloadQuality.defaultOn(platform), platform.value)
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
    fun aValueItDoesntKnowReadsAsTheDefault() = runTest {
        val store = backgroundScope.tempDataStore()
        store.edit {
            it[stringPreferencesKey("kivo.quality.local")] = "bogus"
            it[stringPreferencesKey("kivo.audio.passthrough")] = "On"
            it[intPreferencesKey("kivo.audio.maxChannels")] = 4
            it[stringPreferencesKey("kivo.quality.home")] = "8000000"
        }
        assertEquals(DeviceSettings(), DeviceSettingsStore(store).values.first())
    }

    @Test
    fun qualitiesBecomeRequests() {
        assertEquals(QualityRequest("auto", bandwidthBps = 50_000_000), Quality.Auto.request(50_000_000))
        assertEquals(QualityRequest("original"), Quality.Original.request())
        assertEquals(QualityRequest("limit", maxBitrateBps = 8_000_000), Quality.Limit(8_000_000).request())
    }
}
