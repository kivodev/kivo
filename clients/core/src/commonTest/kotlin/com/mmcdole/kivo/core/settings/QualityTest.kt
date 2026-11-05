package com.mmcdole.kivo.core.settings

import com.mmcdole.kivo.api.models.QualityRequest
import com.mmcdole.kivo.api.models.QualityRung
import kotlin.test.Test
import kotlin.test.assertEquals

class QualityTest {
    @Test
    fun aQualityTheServerDoesntOfferIsAuto() {
        val rungs = listOf(QualityRung(8_000_000, 1080), QualityRung(4_000_000, 720))
        assertEquals(Quality.Limit(4_000_000), Quality.Limit(4_000_000).onLadder(rungs))
        assertEquals(Quality.Auto, Quality.Limit(3_000_000).onLadder(rungs))
        assertEquals(Quality.Original, Quality.Original.onLadder(rungs))

        val settings = DeviceSettings(localQuality = Quality.Limit(20_000_000), remoteQuality = Quality.Limit(8_000_000))
        assertEquals(DeviceSettings(localQuality = Quality.Auto, remoteQuality = Quality.Limit(8_000_000)), settings.onLadder(rungs))
    }

    @Test
    fun aQualityGoesByItsKeyAndAsAPlaySaysIt() {
        for (quality in listOf(Quality.Auto, Quality.Original, Quality.Limit(8_000_000))) {
            assertEquals(quality, Quality.of(quality.key))
            assertEquals(quality, Quality.of(quality.request(bandwidthBps = 5_000_000)))
        }
        assertEquals(Quality.Auto, Quality.of(null as String?))
        assertEquals(Quality.Auto, Quality.of("fast"))
        assertEquals(Quality.Auto, Quality.of(null as QualityRequest?), "a play that says none plays at Auto")
    }
}
