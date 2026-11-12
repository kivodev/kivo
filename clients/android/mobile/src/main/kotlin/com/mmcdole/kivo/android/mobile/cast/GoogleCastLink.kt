package com.mmcdole.kivo.android.mobile.cast

import android.content.Context
import androidx.core.net.toUri
import com.google.android.gms.cast.MediaInfo
import com.google.android.gms.cast.MediaLoadRequestData
import com.google.android.gms.cast.MediaMetadata
import com.google.android.gms.cast.MediaSeekOptions
import com.google.android.gms.cast.MediaStatus
import com.google.android.gms.cast.framework.CastContext
import com.google.android.gms.cast.framework.CastSession
import com.google.android.gms.cast.framework.CastState
import com.google.android.gms.cast.framework.CastStateListener
import com.google.android.gms.cast.framework.SessionManagerListener
import com.google.android.gms.cast.framework.media.RemoteMediaClient
import com.google.android.gms.common.ConnectionResult
import com.google.android.gms.common.GoogleApiAvailability
import com.google.android.gms.common.images.WebImage
import com.mmcdole.kivo.core.cast.CAST_NAMESPACE
import com.mmcdole.kivo.core.cast.CastLink
import com.mmcdole.kivo.core.cast.CastLinkState
import com.mmcdole.kivo.core.cast.CastLoad
import com.mmcdole.kivo.core.cast.CastMedia
import org.json.JSONObject

/**
 * Google Cast: the shared context, set up with Kivo's receiver
 * ([CastSetup]), its session manager and the session's media client, and
 * Kivo's namespace. Made only where Google Play services is ([available]).
 */
class GoogleCastLink private constructor(private val context: CastContext) : CastLink {
    private var events: CastLink.Events? = null
    private var session: CastSession? = null
    private val client: RemoteMediaClient? get() = session?.remoteMediaClient

    private val castState = CastStateListener { changed(it) }

    private val mediaCallback = object : RemoteMediaClient.Callback() {
        override fun onStatusUpdated() = mediaChanged()

        override fun onMetadataUpdated() = mediaChanged()
    }

    private val castListener = object : com.google.android.gms.cast.Cast.Listener() {
        override fun onVolumeChanged() {
            events?.onVolume(session?.volume?.toFloat() ?: 0f)
        }
    }

    private val sessions = object : SessionManagerListener<CastSession> {
        override fun onSessionStarting(session: CastSession) = report(CastLinkState.Connecting)

        override fun onSessionStarted(session: CastSession, sessionId: String) = attach(session, resumed = false)

        override fun onSessionResumed(session: CastSession, wasSuspended: Boolean) = attach(session, resumed = true)

        override fun onSessionResuming(session: CastSession, sessionId: String) = report(CastLinkState.Connecting)

        override fun onSessionStartFailed(session: CastSession, error: Int) = detach()

        override fun onSessionResumeFailed(session: CastSession, error: Int) = detach()

        override fun onSessionEnding(session: CastSession) = Unit

        override fun onSessionEnded(session: CastSession, error: Int) = detach()

        override fun onSessionSuspended(session: CastSession, reason: Int) = report(CastLinkState.Connecting)
    }

    override val positionMs: Long get() = client?.approximateStreamPosition ?: 0

    override fun start(events: CastLink.Events) {
        this.events = events
        context.addCastStateListener(castState)
        context.sessionManager.addSessionManagerListener(sessions, CastSession::class.java)
        context.sessionManager.currentCastSession?.takeIf { it.isConnected }?.let { attach(it, resumed = true) } ?: changed(context.castState)
    }

    override fun load(load: CastLoad, title: String, imageUrl: String?) {
        val client = client ?: return
        val metadata = MediaMetadata(MediaMetadata.MEDIA_TYPE_MOVIE).apply {
            putString(MediaMetadata.KEY_TITLE, title)
            imageUrl?.let { addImage(WebImage(it.toUri(), 480, 720)) }
        }
        val media = MediaInfo.Builder(load.contentId)
            .setEntity(load.entity)
            .setStreamType(MediaInfo.STREAM_TYPE_BUFFERED)
            .setMetadata(metadata)
            .setCustomData(JSONObject(load.customData))
            .build()
        val request = MediaLoadRequestData.Builder()
            .setMediaInfo(media)
            .setAutoplay(true)
            .apply { load.currentTimeSec?.let { setCurrentTime((it * 1000).toLong()) } }
            .build()
        client.load(request)
    }

    override fun play() {
        client?.play()
    }

    override fun pause() {
        client?.pause()
    }

    override fun seek(positionMs: Long) {
        client?.seek(MediaSeekOptions.Builder().setPosition(positionMs).build())
    }

    override fun setSpeed(speed: Float) {
        client?.setPlaybackRate(speed.toDouble())
    }

    override fun setVolume(volume: Float) {
        session?.volume = volume.toDouble()
    }

    override fun send(message: String) {
        session?.sendMessage(CAST_NAMESPACE, message)
    }

    override fun end(stopCasting: Boolean) {
        context.sessionManager.endCurrentSession(stopCasting)
    }

    override fun close() {
        events = null
        context.removeCastStateListener(castState)
        context.sessionManager.removeSessionManagerListener(sessions, CastSession::class.java)
        session?.let { detachClient(it) }
    }

    private fun report(state: CastLinkState) {
        events?.onLink(state)
    }

    private fun changed(castState: Int) {
        if (session != null) return
        report(
            when (castState) {
                CastState.NO_DEVICES_AVAILABLE -> CastLinkState.Unavailable
                CastState.CONNECTING -> CastLinkState.Connecting
                else -> CastLinkState.Disconnected
            },
        )
    }

    private fun attach(session: CastSession, resumed: Boolean) {
        this.session?.let { detachClient(it) }
        this.session = session
        session.setMessageReceivedCallbacks(CAST_NAMESPACE) { _, _, message -> events?.onMessage(message) }
        session.addCastListener(castListener)
        session.remoteMediaClient?.registerCallback(mediaCallback)
        events?.onVolume(session.volume.toFloat())
        report(CastLinkState.Connected(session.castDevice?.friendlyName.orEmpty(), resumed))
        mediaChanged()
    }

    private fun detach() {
        session?.let { detachClient(it) }
        session = null
        events?.onMedia(null)
        changed(context.castState)
    }

    private fun detachClient(session: CastSession) {
        session.remoteMediaClient?.unregisterCallback(mediaCallback)
        session.removeCastListener(castListener)
        runCatching { session.removeMessageReceivedCallbacks(CAST_NAMESPACE) }
    }

    private fun mediaChanged() {
        val status = client?.mediaStatus
        val info = status?.mediaInfo
        val workId = info?.contentId
        if (status == null || workId == null) {
            events?.onMedia(null)
            return
        }
        val state = when (status.playerState) {
            MediaStatus.PLAYER_STATE_PLAYING -> CastMedia.State.Playing
            MediaStatus.PLAYER_STATE_PAUSED -> CastMedia.State.Paused
            MediaStatus.PLAYER_STATE_BUFFERING -> CastMedia.State.Buffering
            MediaStatus.PLAYER_STATE_LOADING -> CastMedia.State.Loading
            // Idle with media is a load under way; finished, stopped or failed leaves nothing playing.
            else -> if (status.idleReason == MediaStatus.IDLE_REASON_NONE) {
                CastMedia.State.Loading
            } else {
                events?.onMedia(null)
                return
            }
        }
        events?.onMedia(
            CastMedia(workId, state, status.streamPosition, info.streamDuration.coerceAtLeast(0), status.playbackRate.toFloat(), status.customData?.toString()),
        )
    }

    companion object {
        /**
         * Google Cast on this phone, or null where it can't be had: without
         * Google Play services, or with one too old for the Cast framework.
         */
        fun available(context: Context): GoogleCastLink? {
            if (GoogleApiAvailability.getInstance().isGooglePlayServicesAvailable(context) != ConnectionResult.SUCCESS) return null
            @Suppress("DEPRECATION") // The shared context is set up on the main thread, as the app's screens are.
            return runCatching { GoogleCastLink(CastContext.getSharedInstance(context)) }.getOrNull()
        }
    }
}
