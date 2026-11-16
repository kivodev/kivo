package com.mmcdole.kivo.tokens

import java.io.File
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals

class RokuTokensTest {
    private val shared = File("../../shared/tokens.json").readText()

    @Test
    fun coloursAreSceneGraphColours() {
        assertEquals("0x0D0E12FF", rokuColor("#0d0e12"))
        assertEquals("0x0D0E12D9", rokuColor("#0d0e12d9"))
    }

    @Test
    fun theSharedTokensBecomeConstants() {
        val source = rokuTokens(shared)
        assertContains(source, "namespace KivoTokens.Color\n")
        assertContains(source, "    const surfaceRaised = \"0x1D2028FF\"\n")
        assertContains(source, "namespace KivoTokens.TextSize\n")
        assertContains(source, "    const title = 48\n")
        assertContains(source, "    const overscanX = 48\n")
        assertContains(source, "    const posterAcross = 6\n")
        assertContains(source, "    const cardScale = 1.1\n")
        assertContains(source, "    const poster = 0.6667\n")
        // CSS-only values and the light theme have no Roku meaning.
        assertEquals(false, "calc" in source)
        assertEquals(false, "0xF6F7F9FF" in source)
    }

    @Test
    fun theCommittedTokensAreCurrent() {
        assertEquals(rokuTokens(shared), File("../roku/source/generated/tokens.bs").readText(), "run ./gradlew :tokens:rokuTokens")
    }
}
