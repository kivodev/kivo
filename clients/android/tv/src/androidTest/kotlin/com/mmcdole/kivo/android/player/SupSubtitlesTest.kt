package com.mmcdole.kivo.android.player

import android.os.Handler
import android.os.HandlerThread
import androidx.annotation.OptIn
import androidx.core.net.toUri
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.Player
import androidx.media3.common.TrackSelectionOverride
import androidx.media3.common.Tracks
import androidx.media3.common.text.CueGroup
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.DefaultDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.extractor.text.DefaultSubtitleParserFactory
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.After
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlin.test.assertTrue

/**
 * A `.sup` file as the server serves it, side-loaded beside a video as the
 * engine does: pgs-sample.sup (clients/scripts/gen-pgs-sample.py), whose
 * pictures show at 14.1 s (until 15.7 s), 25.2 s and 27.0 s.
 */
@OptIn(UnstableApi::class)
@RunWith(AndroidJUnit4::class)
class SupSubtitlesTest {
    private val thread = HandlerThread("player").apply { start() }
    private val handler = Handler(thread.looper)
    private lateinit var player: ExoPlayer

    /** When each change of cues came, on the player's timeline, and whether it showed a picture. */
    private val shown = CopyOnWriteArrayList<Pair<Long, Boolean>>()

    @After
    fun release() {
        val done = CountDownLatch(1)
        handler.post {
            player.release()
            done.countDown()
        }
        done.await(5, TimeUnit.SECONDS)
        thread.quitSafely()
    }

    @Test
    fun eachPictureShowsAtItsTimeAndClearsAtTheNext() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val reached = CountDownLatch(1)
        // The test's assets, copied where the app's player reads files.
        val (video, subs) = listOf("video.mp4", "pgs-sample.sup").map { name ->
            File(instrumentation.targetContext.cacheDir, name).apply {
                instrumentation.context.assets.open("pgs/$name").use { input -> outputStream().use { input.copyTo(it) } }
            }
        }
        handler.post {
            val sources = DefaultMediaSourceFactory(DefaultDataSource.Factory(instrumentation.targetContext))
                .setSubtitleParserFactory(SupParser.Factory(DefaultSubtitleParserFactory()))
            player = ExoPlayer.Builder(instrumentation.targetContext).setLooper(thread.looper).setMediaSourceFactory(sources).build()
            player.addListener(object : Player.Listener {
                override fun onTracksChanged(tracks: Tracks) {
                    val text = tracks.groups.firstOrNull { it.type == C.TRACK_TYPE_TEXT } ?: return
                    player.trackSelectionParameters = player.trackSelectionParameters.buildUpon()
                        .setTrackTypeDisabled(C.TRACK_TYPE_TEXT, false)
                        .setOverrideForType(TrackSelectionOverride(text.mediaTrackGroup, 0))
                        .build()
                }

                override fun onCues(cueGroup: CueGroup) {
                    val picture = cueGroup.cues.any { it.bitmap != null && it.bitmap!!.width > 0 }
                    shown += player.currentPosition to picture
                    if (picture && player.currentPosition > 25_000) reached.countDown()
                }
            })
            player.setMediaItem(
                MediaItem.Builder()
                    .setUri(video.toUri())
                    .setSubtitleConfigurations(
                        listOf(
                            MediaItem.SubtitleConfiguration.Builder(subs.toUri())
                                .setId("t3").setMimeType(SUP_MIME).setSelectionFlags(0).build(),
                        ),
                    )
                    .build(),
            )
            player.prepare()
            player.seekTo(13_000)
            player.setPlaybackSpeed(2f)
            player.play()
        }
        assertTrue(reached.await(30, TimeUnit.SECONDS), "no picture past 25 s: $shown")
        val pictures = shown.filter { it.second }.map { it.first }
        assertTrue(pictures.first() in 13_900..14_800, "the first picture at 14.1 s: $shown")
        assertTrue(shown.any { !it.second && it.first in 15_500..16_500 }, "cleared at 15.7 s: $shown")
        assertTrue(pictures.any { it in 25_000..26_000 }, "the next at 25.2 s: $shown")
    }
}
