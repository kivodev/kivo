package com.mmcdole.kivo.core.auth

import com.mmcdole.kivo.core.MemoryPreferences
import com.mmcdole.kivo.core.MemoryStore
import kotlin.test.Test
import kotlin.test.assertEquals

class ServerUrlsTest {
    private fun connection(baseUrl: String) = ServerConnection("server-1", baseUrl, MemoryStore(), MemoryPreferences())

    @Test
    fun theApisUrlsAreRelativeToItsRoot() {
        val c = connection("http://10.0.0.2:8484")
        assertEquals("http://10.0.0.2:8484/api/v1/plays/p1/hls/main.m3u8?t=1", c.apiUrl("plays/p1/hls/main.m3u8?t=1"))
        assertEquals("http://10.0.0.2:8484/fonts/f1", c.apiUrl("/fonts/f1"))
        assertEquals("https://cdn.test/x.webp", c.apiUrl("https://cdn.test/x.webp"))
        assertEquals("http://10.0.0.2:8484/api/v1/images/i1?width=480", c.imageUrl("i1", 480))
        // What the server encoded reaches it as it was.
        assertEquals(
            "http://10.0.0.2:8484/api/v1/fonts/Noto%20Sans.ttf?sig=a+b%2Fc==&x=%C3%A9",
            c.apiUrl("fonts/Noto%20Sans.ttf?sig=a+b%2Fc==&x=%C3%A9"),
        )
    }

    @Test
    fun aServerUnderAPathKeepsIt() {
        val c = connection("https://kivo.example.com/kivo/")
        assertEquals("https://kivo.example.com/kivo/api/v1/images/i1?width=320", c.imageUrl("i1", 320))
        assertEquals("https://kivo.example.com/kivo/api/v1/events", c.apiUrl("events"))
    }
}
