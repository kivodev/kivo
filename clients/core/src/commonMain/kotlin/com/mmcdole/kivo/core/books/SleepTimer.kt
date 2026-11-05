package com.mmcdole.kivo.core.books

/**
 * The sleep timer: after a number of minutes of listening, or at the end of
 * the chapter, the book fades out over 10 seconds and pauses, keeping its
 * place. Minutes count down only while the book plays, so a pause doesn't
 * spend them; the end of the chapter is measured from the position at the
 * speed playing. Choosing again starts it afresh, and Off stops it, either
 * one undoing a fade under way. The player [tick]s it several times a second
 * while it runs and does what it says.
 */
internal class SleepTimer {
    var state: SleepState = SleepState.Off
        private set

    /** The last choice, which the menu offers first. */
    var last: SleepChoice? = null
        private set

    private var remainingMs = 0L
    private var fading = false

    /** Starts the timer afresh. Returns a command when a fade has to be undone. */
    fun set(choice: SleepChoice, chapterLeftMs: Long = 0, speed: Double = 1.0): SleepCommand? {
        val undo = stopFade()
        last = choice
        remainingMs = when (choice) {
            is SleepChoice.Minutes -> choice.minutes * 60_000L
            SleepChoice.EndOfChapter -> wallMs(chapterLeftMs, speed)
        }
        state = SleepState.Running(choice, remainingMs, fading = false)
        return undo
    }

    /** Stops the timer. Returns a command when a fade has to be undone. */
    fun off(): SleepCommand? {
        state = SleepState.Off
        return stopFade()
    }

    /**
     * [elapsedMs] passed on the clock since the last tick, [playing] or
     * not; for End of chapter, [chapterLeftMs] of the chapter is left to
     * play at [speed]. Returns what the audio must do now, if anything.
     */
    fun tick(elapsedMs: Long, playing: Boolean, chapterLeftMs: Long = 0, speed: Double = 1.0): SleepCommand? {
        val running = state as? SleepState.Running ?: return null
        remainingMs = when (running.choice) {
            is SleepChoice.Minutes -> if (playing) remainingMs - elapsedMs else remainingMs
            SleepChoice.EndOfChapter -> wallMs(chapterLeftMs, speed)
        }.coerceAtLeast(0)
        val command = when {
            !playing -> if (fading) SleepCommand.RestoreVolume.also { fading = false } else null
            remainingMs <= 0 -> SleepCommand.Pause
            remainingMs <= SLEEP_FADE_MS && !fading -> SleepCommand.FadeOut(remainingMs).also { fading = true }
            else -> null
        }
        state = if (command == SleepCommand.Pause) {
            fading = false
            SleepState.Off
        } else {
            SleepState.Running(running.choice, remainingMs, fading)
        }
        return command
    }

    private fun stopFade(): SleepCommand? {
        val undo = if (fading) SleepCommand.RestoreVolume else null
        fading = false
        return undo
    }

    private fun wallMs(mediaMs: Long, speed: Double): Long = (mediaMs / speed.coerceAtLeast(0.1)).toLong().coerceAtLeast(0)
}

/** What the sleep timer was set to. */
sealed class SleepChoice {
    data class Minutes(val minutes: Int) : SleepChoice()

    data object EndOfChapter : SleepChoice()

    companion object {
        /** The sleep timer's lengths in minutes, before "End of chapter". */
        val MINUTES: List<Int> = listOf(5, 10, 15, 30, 45, 60)
    }
}

/** The sleep timer as the Sleep button shows it. */
sealed class SleepState {
    data object Off : SleepState()

    /**
     * Running: about [remainingMs] of listening left, in time on the clock
     * (a chapter's end comes sooner at a higher speed), [fading] in its last
     * 10 seconds.
     */
    data class Running(val choice: SleepChoice, val remainingMs: Long, val fading: Boolean) : SleepState()
}

/** What the sleep timer has the platform do to the audio; the platform does it. */
internal sealed interface SleepCommand {
    /** Lower the volume to silence over [durationMs]. */
    data class FadeOut(val durationMs: Long) : SleepCommand

    /** Back to full volume: the fade stopped short. */
    data object RestoreVolume : SleepCommand

    /** Time's up: pause, then put the volume back for next time. */
    data object Pause : SleepCommand
}

/** How long the book fades out before the sleep timer pauses it. */
internal const val SLEEP_FADE_MS = 10_000L
