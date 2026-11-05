package com.mmcdole.kivo.tokens

import java.io.File
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class ComposeTokensTest {
    private val shared = File("../../shared/tokens.json").readText()

    @Test
    fun namesAreKotlin() {
        assertEquals("surfaceRaised", name("surface-raised"))
        assertEquals("onPlayerMuted", name("on-player-muted"))
        assertEquals("xl2", name("2xl"))
        assertEquals("bg", name("bg"))
    }

    @Test
    fun coloursKeepTheirAlpha() {
        assertEquals("Color(0xFF0D0E12)", color("#0d0e12"))
        assertEquals("Color(0xD90D0E12)", color("#0d0e12d9"))
    }

    @Test
    fun theSharedTokensBecomeComposeValues() {
        val source = composeTokens(shared, "com.example")
        assertContains(source, "package com.example")
        assertContains(source, "val surfaceRaised: Color,")
        assertContains(source, "bg = Color(0xFF0D0E12),")
        // A rem is 16 dp; TV pixels on a 1080p canvas are half a dp.
        assertContains(source, "val spacing = 4.dp")
        assertContains(source, "val sidebarWidth = 240.dp")
        assertContains(source, "val base = TextStep(16.sp, 24.sp)")
        assertContains(source, "val base = TextStep(14.sp, 20.sp)")
        assertContains(source, "val focusRingWidth = 2.dp")
        assertContains(source, "const val poster = 0.6666667f")
        assertContains(source, "const val fastMs = 150")
        assertContains(source, "val standard = CubicBezierEasing(0.2f, 0.0f, 0.0f, 1.0f)")
        // A grid template isn't a length.
        assertEquals(false, "posterGrid" in source)
    }

    @Test
    fun aLightThemeMissingAColourIsRefused() {
        val broken = shared.replace("\"surface-raised\": \"#ffffff\",", "")
        assertFailsWith<IllegalArgumentException> { composeTokens(broken, "com.example") }
    }
}
