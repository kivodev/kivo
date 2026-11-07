package com.mmcdole.kivo.android

import kotlin.test.Test
import kotlin.test.assertEquals

class DeviceNameTest {
    @Test
    fun theNameSetInTheSystemsSettingsComesFirst() {
        assertEquals("Living Room", deviceName("Living Room", "Living Room TV", "sdk_google_atv64_x86_64"))
    }

    @Test
    fun withoutOneTheBluetoothNameThenTheModel() {
        assertEquals("Bedroom", deviceName(null, "Bedroom", "Chromecast"))
        assertEquals("Bedroom", deviceName(" ", "Bedroom", "Chromecast"))
        assertEquals("Chromecast", deviceName(null, "", "Chromecast"))
    }
}
