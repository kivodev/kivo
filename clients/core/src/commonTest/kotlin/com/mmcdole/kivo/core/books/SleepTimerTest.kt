package com.mmcdole.kivo.core.books

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class SleepTimerTest {
    private fun SleepTimer.remaining() = (state as SleepState.Running).remainingMs

    @Test
    fun minutesCountDownWhilePlayingThenFadeOverTheLastTenSecondsAndPause() {
        val timer = SleepTimer()
        assertNull(timer.set(SleepChoice.Minutes(5)))
        assertEquals(300_000, timer.remaining())

        assertNull(timer.tick(60_000, playing = false), "a pause doesn't spend it")
        assertEquals(300_000, timer.remaining())
        assertNull(timer.tick(289_000, playing = true))
        assertEquals(11_000, timer.remaining())

        assertEquals(SleepCommand.FadeOut(10_000), timer.tick(1_000, playing = true), "the fade starts 10 s before the end")
        assertEquals(SleepState.Running(SleepChoice.Minutes(5), 10_000, fading = true), timer.state)
        assertNull(timer.tick(9_750, playing = true), "fading once")
        assertEquals(SleepCommand.Pause, timer.tick(250, playing = true))
        assertEquals(SleepState.Off, timer.state)
        assertNull(timer.tick(250, playing = true), "off, it does nothing")
    }

    @Test
    fun pausingDuringTheFadeBringsTheVolumeBackAndPlayingFadesWhatsLeft() {
        val timer = SleepTimer()
        timer.set(SleepChoice.Minutes(5))
        assertNull(timer.tick(289_000, playing = true))
        assertEquals(SleepCommand.FadeOut(6_000), timer.tick(5_000, playing = true))
        assertEquals(SleepCommand.RestoreVolume, timer.tick(2_000, playing = false))
        assertEquals(6_000, timer.remaining(), "paused, nothing is spent")
        assertEquals(SleepCommand.FadeOut(5_750), timer.tick(250, playing = true))
    }

    @Test
    fun offAndChoosingAgainUndoAFadeAndStartAfresh() {
        val timer = SleepTimer()
        timer.set(SleepChoice.Minutes(10))
        timer.tick(595_000, playing = true)
        assertEquals(SleepState.Running(SleepChoice.Minutes(10), 5_000, fading = true), timer.state)

        assertEquals(SleepCommand.RestoreVolume, timer.set(SleepChoice.Minutes(15)), "extended in the fade")
        assertEquals(SleepState.Running(SleepChoice.Minutes(15), 900_000, fading = false), timer.state)

        timer.tick(895_000, playing = true)
        assertEquals(SleepCommand.RestoreVolume, timer.off(), "cancelled in the fade")
        assertEquals(SleepState.Off, timer.state)
        assertNull(timer.off(), "nothing to undo")
        assertEquals(SleepChoice.Minutes(15), timer.last, "the last choice, offered first")
    }

    @Test
    fun endOfChapterIsMeasuredFromThePositionAtTheSpeedPlaying() {
        val timer = SleepTimer()
        timer.set(SleepChoice.EndOfChapter, chapterLeftMs = 120_000, speed = 2.0)
        assertEquals(60_000, timer.remaining(), "at 2×, two minutes of chapter is one on the clock")
        assertNull(timer.tick(250, playing = true, chapterLeftMs = 21_000, speed = 2.0))
        assertEquals(SleepCommand.FadeOut(10_000), timer.tick(250, playing = true, chapterLeftMs = 20_000, speed = 2.0))
        assertNull(timer.tick(250, playing = true, chapterLeftMs = 9_000, speed = 1.0))
        assertEquals(SleepCommand.Pause, timer.tick(250, playing = true, chapterLeftMs = -100, speed = 1.0), "past the end")
        assertEquals(SleepState.Off, timer.state)
    }
}
