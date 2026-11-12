package com.mmcdole.kivo.tokens

import java.io.File
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class SwiftTokensTest {
    private val shared = File("../../shared/tokens.json").readText()

    @Test
    fun coloursAreSRGBComponentsWithTheirAlpha() {
        assertEquals("Color(.sRGB, red: 0.0510, green: 0.0549, blue: 0.0706, opacity: 1)", swiftColor("#0d0e12"))
        assertEquals("Color(.sRGB, red: 0.0510, green: 0.0549, blue: 0.0706, opacity: 0.8510)", swiftColor("#0d0e12d9"))
    }

    @Test
    fun shadowLayersHalveTheBlur() {
        assertEquals(
            "[KivoShadow(color: Color(.sRGB, red: 0.0000, green: 0.0000, blue: 0.0000, opacity: 0.4000), radius: 4, x: 0, y: 2)]",
            swiftShadow("0 2px 8px #00000066"),
        )
        assertEquals(2, swiftShadow("0 0 4px #000000, 0 1px 2px #000000").split("KivoShadow(").size - 1)
    }

    @Test
    fun theSharedTokensBecomeSwiftUIValues() {
        val source = swiftTokens(shared)
        assertContains(source, "import SwiftUI")
        assertContains(source, "    public let surfaceRaised: Color")
        assertContains(source, "    public let cardShadow: [KivoShadow]")
        assertContains(source, "static let dark = KivoPalette(")
        assertContains(source, "        scheme: .light,")
        // A rem is 16 points.
        assertContains(source, "public static let spacing: CGFloat = 4")
        assertContains(source, "public static let sidebarWidth: CGFloat = 240")
        assertContains(source, "public static let base = KivoTextStyle(size: 16, lineHeight: 24)")
        assertContains(source, "public static let xl2 = KivoTextStyle(size: 28, lineHeight: 36)")
        assertContains(source, "public static let poster: CGFloat = 2.0 / 3.0")
        assertEquals(false, "posterGrid" in source)
        assertEquals(false, "enum Tv" in source)
    }

    @Test
    fun aThemeMissingAColourIsRefused() {
        val broken = shared.replace("\"surface-raised\": \"#ffffff\",", "")
        assertFailsWith<IllegalArgumentException> { swiftTokens(broken) }
    }
}
