package com.mmcdole.kivo.android.detail

import kotlin.test.Test
import kotlin.test.assertEquals

class HeroLevelsTest {
    @Test
    fun upReturnsToTheButtonYouCameFrom() {
        assertEquals("played", returningButton("played", setOf("play", "favorite", "played")))
    }

    @Test
    fun upReturnsToPlayUntilThereHasBeenAButton() {
        assertEquals("play", returningButton("play", setOf("play", "favorite")))
    }

    @Test
    fun aButtonThatHasGoneGivesWayToPlay() {
        assertEquals("play", returningButton("restart", setOf("play", "favorite")))
    }

    @Test
    fun withoutPlayFavoriteTakesItsPlace() {
        assertEquals("favorite", returningButton("play", setOf("favorite", "played")))
    }
}
