package com.mmcdole.kivo.android.detail

import kotlin.test.Test
import kotlin.test.assertEquals

class HeroLevelsTest {
    @Test
    fun upReturnsToTheButtonYouCameFrom() {
        assertEquals("played", returningButton("played", setOf("play", "myList", "played")))
    }

    @Test
    fun upReturnsToPlayUntilThereHasBeenAButton() {
        assertEquals("play", returningButton("play", setOf("play", "myList")))
    }

    @Test
    fun aButtonThatHasGoneGivesWayToPlay() {
        assertEquals("play", returningButton("restart", setOf("play", "myList")))
    }

    @Test
    fun withoutPlayMyListTakesItsPlace() {
        assertEquals("myList", returningButton("play", setOf("myList", "played")))
    }
}
