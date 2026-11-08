package com.mmcdole.kivo.android

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.media3.common.Player
import androidx.media3.session.MediaSession
import androidx.media3.session.MediaSessionService
import kotlinx.coroutines.MainScope
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

/**
 * The phone's playback in the background: one media session over the
 * player the app's [com.mmcdole.kivo.android.player.SessionPlayers] holds,
 * the video's or the book's, so its notification, the lock screen,
 * headphones and Bluetooth controls reach it, and the app stays in the
 * foreground's priority while it plays. It starts as a player is offered
 * ([start]) and stops once none is left. A TV's players keep sessions of
 * their own.
 */
class PlaybackService : MediaSessionService() {
    private val scope = MainScope()
    private var session: MediaSession? = null

    override fun onCreate() {
        super.onCreate()
        val players = (application as HasAppGraph).graph.sessionPlayers
        scope.launch { players.current.collect(::follow) }
    }

    override fun onGetSession(controllerInfo: MediaSession.ControllerInfo): MediaSession? = session

    /** Swiped away from recents while paused: nothing is left to keep going. */
    override fun onTaskRemoved(rootIntent: Intent?) {
        val player = session?.player
        if (player == null || !player.playWhenReady || player.mediaItemCount == 0) stopSelf()
    }

    override fun onDestroy() {
        scope.cancel()
        session?.release()
        session = null
        super.onDestroy()
    }

    private fun follow(player: Player?) {
        val current = session
        when {
            player == null -> {
                if (current != null) {
                    removeSession(current)
                    current.release()
                    session = null
                }
                stopSelf()
            }
            current == null -> session = MediaSession.Builder(this, player).setSessionActivity(openApp()).build().also(::addSession)
            current.player !== player -> current.player = player
        }
    }

    /** The notification opens the app where it was. */
    private fun openApp(): PendingIntent = PendingIntent.getActivity(
        this,
        0,
        Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
        PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
    )

    companion object {
        /** Starts the service, as a player is offered with the app in the foreground. */
        fun start(context: Context) {
            try {
                context.startService(Intent(context, PlaybackService::class.java))
            } catch (_: IllegalStateException) {
                // In the background: the session that's already running keeps the player.
            }
        }
    }
}
