package com.mmcdole.kivo.core.cast

/**
 * The Cast session each phone implements over Google's Cast SDK, driven by
 * the session's cast remote: the session with a Cast device, the receiver's
 * media and volume, and Kivo's messages on [CAST_NAMESPACE].
 */
interface CastLink {
    /** Where the receiver's media is now, between status updates. */
    val positionMs: Long

    /** Reports to [events] from now until [close]: first the session as it is, then each change. */
    fun start(events: Events)

    /** Loads [load] on the receiver; [imageUrl] is the work's artwork, for Google Home and the cast notification. */
    fun load(load: CastLoad, title: String, imageUrl: String?)

    fun play()

    fun pause()

    fun seek(positionMs: Long)

    fun setSpeed(speed: Float)

    /** The TV's volume, 0 to 1. */
    fun setVolume(volume: Float)

    /** A message on [CAST_NAMESPACE]. */
    fun send(message: String)

    /** Ends the session: [stopCasting] stops the TV too, else it plays on. */
    fun end(stopCasting: Boolean)

    /** Stops reporting; the session carries on. */
    fun close()

    /** What the link tells the remote, on the main thread. */
    interface Events {
        fun onLink(state: CastLinkState)

        /** The receiver's media; null with nothing loaded, or once it finished, stopped or failed. */
        fun onMedia(media: CastMedia?)

        /** The TV's volume, 0 to 1; 0 while muted. */
        fun onVolume(volume: Float)

        /** A message on [CAST_NAMESPACE]. */
        fun onMessage(message: String)
    }
}

/** Whether Cast devices are found, and the session with one. */
sealed interface CastLinkState {
    /** None found, as off Wi-Fi: no Cast button shows. */
    data object Unavailable : CastLinkState

    data object Disconnected : CastLinkState

    data object Connecting : CastLinkState

    /** Connected to [device], such as "Living Room TV": newly, or joined again after the app was away when [resumed]. */
    data class Connected(val device: String, val resumed: Boolean) : CastLinkState
}

/** The receiver's media, by its media status. */
data class CastMedia(
    /** The media's content ID: the work's. */
    val workId: String,
    val state: State,
    val positionMs: Long,
    /** 0 while the receiver doesn't know it. */
    val durationMs: Long,
    val speed: Float,
    /** The media status' custom data, as JSON, which holds the receiver's status. */
    val status: String?,
) {
    enum class State { Loading, Buffering, Playing, Paused }
}
