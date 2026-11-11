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
import com.mmcdole.kivo.core.cast.CastLoad
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.json.JSONObject

/** Whether Cast devices are found, and the session with one. */
sealed interface LinkState {
    /** None found, as off Wi-Fi. */
    data object Unavailable : LinkState

    data object Disconnected : LinkState

    data object Connecting : LinkState

    /** Connected to [device], newly or again after the app was away: [resumed] says which. */
    data class Connected(val device: String, val resumed: Boolean) : LinkState
}

/** The receiver's media: what plays, where, and its `customData`, as JSON, which the core reads. */
data class CastMedia(val workId: String, val state: State, val positionMs: Long, val durationMs: Long, val status: String?) {
    enum class State { Loading, Buffering, Playing, Paused }
}

/** The Cast session as the cast model drives it: Google's SDK, or a test's stand-in. */
interface CastLink {
    val state: StateFlow<LinkState>
    val media: StateFlow<CastMedia?>

    /** The TV's volume, 0 to 1. */
    val volume: StateFlow<Float>

    /** Messages on Kivo's namespace. */
    val messages: SharedFlow<String>

    /** Where the receiver's media is now, between status updates. */
    val positionMs: Long

    /** [imageUrl] is the work's artwork, for Google Home and the cast notification. */
    fun load(load: CastLoad, title: String, imageUrl: String?)

    fun play()

    fun pause()

    fun seek(positionMs: Long)

    fun setVolume(volume: Float)

    fun send(message: String)

    /** Ends the session: [stopCasting] stops the TV too, else it plays on. */
    fun end(stopCasting: Boolean)

    /** Stops following the session, which carries on: the app signed out. */
    fun close()
}

/**
 * Google Cast: the shared context, set up with Kivo's receiver
 * ([CastOptions]), its session manager and the session's media client, and
 * Kivo's namespace. Made only where Google Play services is ([available]).
 */
class GoogleCastLink private constructor(private val context: CastContext) : CastLink {
    private val mutableState = MutableStateFlow<LinkState>(LinkState.Unavailable)
    override val state: StateFlow<LinkState> = mutableState.asStateFlow()
    private val mutableMedia = MutableStateFlow<CastMedia?>(null)
    override val media: StateFlow<CastMedia?> = mutableMedia.asStateFlow()
    private val mutableVolume = MutableStateFlow(0f)
    override val volume: StateFlow<Float> = mutableVolume.asStateFlow()
    override val messages = MutableSharedFlow<String>(extraBufferCapacity = 8)

    private var session: CastSession? = null
    private val client: RemoteMediaClient? get() = session?.remoteMediaClient

    private val castState = CastStateListener { changed(it) }

    private val mediaCallback = object : RemoteMediaClient.Callback() {
        override fun onStatusUpdated() = mediaChanged()

        override fun onMetadataUpdated() = mediaChanged()
    }

    private val castListener = object : com.google.android.gms.cast.Cast.Listener() {
        override fun onVolumeChanged() {
            mutableVolume.value = session?.volume?.toFloat() ?: 0f
        }
    }

    private val sessions = object : SessionManagerListener<CastSession> {
        override fun onSessionStarting(session: CastSession) {
            mutableState.value = LinkState.Connecting
        }

        override fun onSessionStarted(session: CastSession, sessionId: String) = attach(session, resumed = false)

        override fun onSessionResumed(session: CastSession, wasSuspended: Boolean) = attach(session, resumed = true)

        override fun onSessionResuming(session: CastSession, sessionId: String) {
            mutableState.value = LinkState.Connecting
        }

        override fun onSessionStartFailed(session: CastSession, error: Int) = detach()

        override fun onSessionResumeFailed(session: CastSession, error: Int) = detach()

        override fun onSessionEnding(session: CastSession) = Unit

        override fun onSessionEnded(session: CastSession, error: Int) = detach()

        override fun onSessionSuspended(session: CastSession, reason: Int) {
            mutableState.value = LinkState.Connecting
        }
    }

    init {
        context.addCastStateListener(castState)
        context.sessionManager.addSessionManagerListener(sessions, CastSession::class.java)
        context.sessionManager.currentCastSession?.takeIf { it.isConnected }?.let { attach(it, resumed = true) } ?: changed(context.castState)
    }

    override val positionMs: Long get() = client?.approximateStreamPosition ?: 0

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
        context.removeCastStateListener(castState)
        context.sessionManager.removeSessionManagerListener(sessions, CastSession::class.java)
        client?.unregisterCallback(mediaCallback)
        session?.removeCastListener(castListener)
    }

    private fun changed(castState: Int) {
        if (session != null) return
        mutableState.value = when (castState) {
            CastState.NO_DEVICES_AVAILABLE -> LinkState.Unavailable
            CastState.CONNECTING -> LinkState.Connecting
            else -> LinkState.Disconnected
        }
    }

    private fun attach(session: CastSession, resumed: Boolean) {
        this.session?.let { detachClient(it) }
        this.session = session
        session.setMessageReceivedCallbacks(CAST_NAMESPACE) { _, _, message -> messages.tryEmit(message) }
        session.addCastListener(castListener)
        session.remoteMediaClient?.registerCallback(mediaCallback)
        mutableVolume.value = session.volume.toFloat()
        mutableState.value = LinkState.Connected(session.castDevice?.friendlyName.orEmpty(), resumed)
        mediaChanged()
    }

    private fun detach() {
        session?.let { detachClient(it) }
        session = null
        mutableMedia.value = null
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
            mutableMedia.value = null
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
                mutableMedia.value = null
                return
            }
        }
        mutableMedia.value = CastMedia(workId, state, status.streamPosition, info.streamDuration.coerceAtLeast(0), status.customData?.toString())
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
