package com.mmcdole.kivo.core.player

import com.mmcdole.kivo.api.models.PreviewManifest
import com.mmcdole.kivo.core.resolveUrl
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class PreviewsTest {
    private val manifest = PreviewManifest(10_000, 240, 135, 10, 10, listOf("sheet-0.webp", "sheet-1.webp"))
    private val url = "https://kivo.test/api/v1/play-plays/s/previews/manifest.json"

    @Test
    fun findsTheSheetAndTheFramesOffsetForAPosition() {
        assertEquals(SpriteFrame("https://kivo.test/api/v1/play-plays/s/previews/sheet-0.webp", 0, 0, 240, 135), SpriteFrame.atOrNull(manifest, url, 0))
        // Frame 123: the second sheet, row 2, column 3.
        val frame = SpriteFrame.atOrNull(manifest, url, 1_234_000)
        assertEquals("https://kivo.test/api/v1/play-plays/s/previews/sheet-1.webp", frame?.sheetUrl)
        assertEquals(720, frame?.x)
        assertEquals(270, frame?.y)
    }

    @Test
    fun holdsTheLastFramePastTheEndAndSkipsAnEmptyManifest() {
        val frame = SpriteFrame.atOrNull(manifest, url, 99_999_000)
        assertEquals(2160, frame?.x)
        assertEquals(1215, frame?.y)
        assertNull(SpriteFrame.atOrNull(manifest.copy(sheets = emptyList()), url, 0))
    }

    @Test
    fun resolvesSheetsAsALinkWould() {
        assertEquals("https://kivo.test/x.webp", resolveUrl(url, "/x.webp"))
        assertEquals("https://cdn.test/x.webp", resolveUrl(url, "https://cdn.test/x.webp"))
        assertEquals("https://kivo.test/api/v1/play-plays/s/previews/x.webp", resolveUrl("$url?v=2", "x.webp"))
        assertEquals("http://10.0.0.2:8484/kivo/api/v1/images/i1?width=480", resolveUrl("http://10.0.0.2:8484/kivo/api/v1/", "images/i1?width=480"))
    }
}
