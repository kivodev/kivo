package com.mmcdole.kivo.android.ui

import android.view.KeyEvent
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsFocused
import androidx.compose.ui.test.isFocused
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performKeyInput
import androidx.compose.ui.test.pressKey
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.mmcdole.kivo.android.theme.KivoTheme
import com.mmcdole.kivo.api.models.AudioTrackInfo
import com.mmcdole.kivo.api.models.SubtitleTrackInfo
import com.mmcdole.kivo.api.models.Track
import com.mmcdole.kivo.api.models.TrackType
import com.mmcdole.kivo.core.display.Display
import com.mmcdole.kivo.core.player.Subtitles
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import kotlin.test.assertEquals

// Audio & Subtitles on the TV with a film's many tracks: focus starts on the
// audio playing, Right and Left enter the other column on its ticked row, Up
// and Down move within a column, OK chooses, Back closes, and each column
// opens scrolled to its ticked row. Before a play, Automatic heads each
// column, named after what it plays. A searchable version's Subtitles shows
// even without a track, ending in Search subtitles.

@RunWith(AndroidJUnit4::class)
@OptIn(ExperimentalTestApi::class)
class TracksPanelTest {
    @get:Rule
    val compose = createComposeRule()

    private fun audio(id: String, name: String, codec: String, title: String, channels: Int, layout: String, commentary: Boolean = false) =
        Track(id, TrackType.Audio, codec, id == "a1", name, name, language = "eng", title = title, commentary = commentary.takeIf { it }, audio = AudioTrackInfo(channels, layout))

    private fun subtitle(id: String, language: String, title: String? = null) =
        Display().language(language).let { name ->
            Track(id, TrackType.Subtitle, "hdmv_pgs_subtitle", false, name, name, language = language, title = title, subtitle = SubtitleTrackInfo("embedded"))
        }

    private val audio = listOf(
        audio("a1", "English · 5.1", "eac3", "E-AC3 DD+5.1 1024kbs", 6, "5.1"),
        audio("a2", "English · 4.0", "eac3", "E-AC3 DD+4.0 1024kbs Surround Mix", 4, "4.0"),
        audio("a3", "English · 2.0 · FLAC", "flac", "Flac 2.0 Original Stereo Audio 627 kb/s", 2, "2.0"),
        audio("a4", "English · Commentary · 2.0", "ac3", "Commentary by director Ron Underwood", 2, "2.0", commentary = true),
        audio("a5", "English · Commentary · 2.0", "ac3", "Commentary by Jonathan Melville", 2, "2.0", commentary = true),
    )

    private val subtitles = listOf("eng", "eng", "jpn", "fre", "ita", "ger", "spa", "dut", "chi", "dan", "fin", "gre", "nor", "ice", "kor", "por", "swe", "chi")
        .mapIndexed { i, lang -> subtitle("s${i + 1}", lang, listOf("Full", "SDH", null, null, null, null, "Castilian", null, "Simplified").getOrNull(i) ?: "Traditional".takeIf { i == 17 }) }

    private val chosen = mutableListOf<String?>()
    private var closed = false

    private fun open(subtitle: String? = Subtitles.OFF, playing: String? = "a1", automatic: AutomaticTracks? = null) {
        compose.setContent {
            KivoTheme {
                TracksPanel(
                    tracks = TrackChoices(audio, subtitles, playing, subtitle),
                    onAudio = { chosen += it },
                    onSubtitle = { chosen += it },
                    onDismiss = { closed = true },
                    automatic = automatic,
                    dialogueBoost = false.takeIf { automatic == null },
                )
            }
        }
    }

    private fun waitForFocus(tag: String) =
        compose.waitUntil(5_000) { runCatching { compose.onNodeWithTag(tag).assertIsFocused() }.isSuccess }

    private fun press(key: Key, times: Int = 1) {
        repeat(times) {
            compose.onNode(isFocused()).performKeyInput { pressKey(key) }
            compose.waitForIdle()
        }
    }

    @Test
    fun focusStartsOnTheAudioPlayingAndMovesBetweenColumnsOnTheirTickedRows() {
        open()
        waitForFocus("track:a1")
        compose.onNodeWithText("English · 4.0").assertIsDisplayed()
        compose.onNodeWithText("E-AC3 DD+4.0 1024kbs Surround Mix").assertIsDisplayed()

        press(Key.DirectionRight)
        waitForFocus("track:${Subtitles.OFF}")
        press(Key.DirectionDown, 18)
        waitForFocus("track:s18")
        press(Key.DirectionCenter)
        assertEquals(listOf<String?>("s18"), chosen)

        press(Key.DirectionLeft)
        waitForFocus("track:a1")
        press(Key.DirectionDown)
        waitForFocus("track:a2")
        press(Key.DirectionCenter)
        assertEquals(listOf<String?>("s18", "a2"), chosen)
    }

    @Test
    fun theSubtitlesColumnOpensOnItsTickedRow() {
        open(subtitle = "s18")
        waitForFocus("track:a1")
        compose.onNodeWithTag("track:s18").assertIsDisplayed()
        press(Key.DirectionRight)
        waitForFocus("track:s18")
    }

    @Test
    fun beforeAPlayAutomaticHeadsEachColumnNamedAfterWhatItPlays() {
        open(subtitle = null, playing = null, automatic = AutomaticTracks(audio = "a1", subtitle = "s3"))
        waitForFocus("track:automatic-audio")
        compose.onNodeWithText("Automatic (English · 5.1)").assertIsDisplayed()
        compose.onNodeWithText("Automatic (Japanese)").assertIsDisplayed()
        compose.onNodeWithTag("dialogue-boost").assertDoesNotExist()

        press(Key.DirectionDown)
        waitForFocus("track:a1")
        press(Key.DirectionCenter)
        press(Key.DirectionRight)
        waitForFocus("track:automatic-subtitle")
        press(Key.DirectionDown)
        press(Key.DirectionCenter)
        assertEquals(listOf<String?>("a1", Subtitles.OFF), chosen)
    }

    @Test
    fun backCloses() {
        open()
        waitForFocus("track:a1")
        // Back reaches the panel's window as the remote's does, not through a node.
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        instrumentation.sendKeyDownUpSync(KeyEvent.KEYCODE_BACK)
        compose.waitUntil(5_000) { closed }
    }

    @Test
    fun withOneAudioTrackAndNoSubtitlesSubtitlesStillOffersASearch() {
        var searched = false
        compose.setContent {
            KivoTheme {
                TracksPanel(
                    tracks = TrackChoices(audio.take(1), emptyList(), "a1", Subtitles.OFF, searchable = true),
                    onAudio = { chosen += it },
                    onSubtitle = { chosen += it },
                    onDismiss = { closed = true },
                    onSearch = { searched = true },
                )
            }
        }
        waitForFocus("track:${Subtitles.OFF}")
        press(Key.DirectionDown)
        waitForFocus("track:search-subtitles")
        compose.onNodeWithText("Search subtitles…").assertIsDisplayed()
        press(Key.DirectionCenter)
        assertEquals(true, searched)
    }
}
