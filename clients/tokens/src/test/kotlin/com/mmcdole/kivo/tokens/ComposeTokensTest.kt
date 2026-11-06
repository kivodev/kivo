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
    fun aFocusedCardsLightIsTheWebTvs() {
        val source = composeTokens(shared, "com.example")
        assertContains(source, "val cardCaptionShift = 8.dp")
        assertContains(
            source,
            "val glow = listOf(Shadow(radius = 20.dp, color = Color(0x52FFFFFF), spread = 2.dp, offset = DpOffset(0.dp, 0.dp)), " +
                "Shadow(radius = 28.dp, color = Color(0xF2000000), spread = -6.dp, offset = DpOffset(0.dp, 14.dp)))",
        )
        assertContains(source, "val sheen = listOf(Shadow(radius = 24.dp, color = Color(0x2EFFFFFF), spread = -16.dp, offset = DpOffset(0.dp, 32.dp)))")
    }

    @Test
    fun shadowsOfTheWrongKindAreRefused() {
        assertEquals("listOf(Shadow(radius = 2.dp, color = Color(0xFF000000), spread = 0.dp, offset = DpOffset(1.dp, 1.dp)))", shadows("2px 2px 4px #000000", inset = false))
        assertFailsWith<IllegalArgumentException> { shadows("inset 0 2px 4px #000000", inset = false) }
        assertFailsWith<IllegalArgumentException> { shadows("0 2px 4px #000000", inset = true) }
    }

    @Test
    fun aLightThemeMissingAColourIsRefused() {
        val broken = shared.replace("\"surface-raised\": \"#ffffff\",", "")
        assertFailsWith<IllegalArgumentException> { composeTokens(broken, "com.example") }
    }
}
