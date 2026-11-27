package com.mmcdole.kivo.android.ui

import kotlin.test.Test
import kotlin.test.assertEquals

class OverviewCutTest {
    @Test
    fun endsAtTheLastWordThatFitsWithAnEllipsis() {
        val cut = cutAtWord("A spy, a thief and a pilot rob the moon") { it.length <= 16 }
        assertEquals("A spy, a thief…", cut)
    }

    @Test
    fun dropsPunctuationBeforeTheEllipsis() {
        assertEquals("A spy…", cutAtWord("A spy, a thief") { it.length <= 7 })
    }

    @Test
    fun keepsOnlyTheEllipsisWhenNoWordFits() {
        assertEquals("…", cutAtWord("Supercalifragilistic words") { it.length <= 3 })
    }
}
