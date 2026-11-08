package com.mmcdole.kivo.android.player

import androidx.annotation.OptIn
import androidx.media3.common.AudioAttributes
import androidx.media3.common.DeviceInfo
import androidx.media3.common.ForwardingPlayer
import androidx.media3.common.MediaItem
import androidx.media3.common.MediaMetadata
import androidx.media3.common.Metadata
import androidx.media3.common.PlaybackException
import androidx.media3.common.PlaybackParameters
import androidx.media3.common.Player
import androidx.media3.common.Timeline
import androidx.media3.common.TrackSelectionParameters
import androidx.media3.common.Tracks
import androidx.media3.common.VideoSize
import androidx.media3.common.text.CueGroup
import androidx.media3.common.util.UnstableApi

/**
 * The player as the media session offers it: previous and next do what
 * [previous] and [next] say, such as play the episode before and after or
 * move by a book's chapters, offered only while there is one, rather than
 * moving between a play's parts or starting the video over.
 */
@OptIn(UnstableApi::class)
internal class SessionPlayer(
    player: Player,
    private val previous: () -> (() -> Unit)?,
    private val next: () -> (() -> Unit)?,
) : ForwardingPlayer(player) {
    /** The session's listeners, each told the commands this player offers rather than the wrapped player's. */
    private val listeners = mutableMapOf<Player.Listener, Player.Listener>()

    override fun addListener(listener: Player.Listener) {
        val offered = Offered(listener)
        listeners[listener] = offered
        super.addListener(offered)
    }

    override fun removeListener(listener: Player.Listener) {
        listeners.remove(listener)?.let { super.removeListener(it) }
    }

    /** Tells the session what it offers now that an episode either side is known or gone. */
    fun episodesChanged() {
        listeners.values.toList().forEach { it.onAvailableCommandsChanged(availableCommands) }
    }

    override fun getAvailableCommands(): Player.Commands = super.getAvailableCommands().buildUpon()
        .removeAll(*Previous, *Next)
        .addIf(Player.COMMAND_SEEK_TO_PREVIOUS, previous() != null)
        .addIf(Player.COMMAND_SEEK_TO_PREVIOUS_MEDIA_ITEM, previous() != null)
        .addIf(Player.COMMAND_SEEK_TO_NEXT, next() != null)
        .addIf(Player.COMMAND_SEEK_TO_NEXT_MEDIA_ITEM, next() != null)
        .build()

    override fun isCommandAvailable(command: Int): Boolean = when (command) {
        in Previous -> previous() != null
        in Next -> next() != null
        else -> super.isCommandAvailable(command)
    }

    override fun hasPreviousMediaItem(): Boolean = previous() != null

    override fun hasNextMediaItem(): Boolean = next() != null

    override fun seekToPrevious() {
        previous()?.invoke()
    }

    override fun seekToPreviousMediaItem() {
        previous()?.invoke()
    }

    override fun seekToNext() {
        next()?.invoke()
    }

    override fun seekToNextMediaItem() {
        next()?.invoke()
    }

    /**
     * A session's listener, told every event as it is but the commands,
     * which are this player's. Each event is passed on by name: Kotlin's
     * delegation leaves out an interface's default methods, which are all
     * of Player.Listener's.
     */
    @Suppress("OVERRIDE_DEPRECATION", "DEPRECATION")
    private inner class Offered(private val listener: Player.Listener) : Player.Listener {
        override fun onAvailableCommandsChanged(availableCommands: Player.Commands) =
            listener.onAvailableCommandsChanged(this@SessionPlayer.availableCommands)

        override fun onEvents(player: Player, events: Player.Events) = listener.onEvents(player, events)
        override fun onTimelineChanged(timeline: Timeline, reason: Int) = listener.onTimelineChanged(timeline, reason)
        override fun onMediaItemTransition(mediaItem: MediaItem?, reason: Int) = listener.onMediaItemTransition(mediaItem, reason)
        override fun onTracksChanged(tracks: Tracks) = listener.onTracksChanged(tracks)
        override fun onMediaMetadataChanged(mediaMetadata: MediaMetadata) = listener.onMediaMetadataChanged(mediaMetadata)
        override fun onPlaylistMetadataChanged(mediaMetadata: MediaMetadata) = listener.onPlaylistMetadataChanged(mediaMetadata)
        override fun onIsLoadingChanged(isLoading: Boolean) = listener.onIsLoadingChanged(isLoading)
        override fun onLoadingChanged(isLoading: Boolean) = listener.onLoadingChanged(isLoading)
        override fun onTrackSelectionParametersChanged(parameters: TrackSelectionParameters) = listener.onTrackSelectionParametersChanged(parameters)
        override fun onPlayerStateChanged(playWhenReady: Boolean, playbackState: Int) = listener.onPlayerStateChanged(playWhenReady, playbackState)
        override fun onPlaybackStateChanged(playbackState: Int) = listener.onPlaybackStateChanged(playbackState)
        override fun onPlayWhenReadyChanged(playWhenReady: Boolean, reason: Int) = listener.onPlayWhenReadyChanged(playWhenReady, reason)
        override fun onPlaybackSuppressionReasonChanged(reason: Int) = listener.onPlaybackSuppressionReasonChanged(reason)
        override fun onIsPlayingChanged(isPlaying: Boolean) = listener.onIsPlayingChanged(isPlaying)
        override fun onRepeatModeChanged(repeatMode: Int) = listener.onRepeatModeChanged(repeatMode)
        override fun onShuffleModeEnabledChanged(shuffleModeEnabled: Boolean) = listener.onShuffleModeEnabledChanged(shuffleModeEnabled)
        override fun onPlayerError(error: PlaybackException) = listener.onPlayerError(error)
        override fun onPlayerErrorChanged(error: PlaybackException?) = listener.onPlayerErrorChanged(error)
        override fun onPositionDiscontinuity(reason: Int) = listener.onPositionDiscontinuity(reason)
        override fun onPositionDiscontinuity(oldPosition: Player.PositionInfo, newPosition: Player.PositionInfo, reason: Int) =
            listener.onPositionDiscontinuity(oldPosition, newPosition, reason)
        override fun onPlaybackParametersChanged(playbackParameters: PlaybackParameters) = listener.onPlaybackParametersChanged(playbackParameters)
        override fun onSeekBackIncrementChanged(seekBackIncrementMs: Long) = listener.onSeekBackIncrementChanged(seekBackIncrementMs)
        override fun onSeekForwardIncrementChanged(seekForwardIncrementMs: Long) = listener.onSeekForwardIncrementChanged(seekForwardIncrementMs)
        override fun onMaxSeekToPreviousPositionChanged(maxSeekToPreviousPositionMs: Long) =
            listener.onMaxSeekToPreviousPositionChanged(maxSeekToPreviousPositionMs)
        override fun onAudioSessionIdChanged(audioSessionId: Int) = listener.onAudioSessionIdChanged(audioSessionId)
        override fun onAudioAttributesChanged(audioAttributes: AudioAttributes) = listener.onAudioAttributesChanged(audioAttributes)
        override fun onVolumeChanged(volume: Float) = listener.onVolumeChanged(volume)
        override fun onSkipSilenceEnabledChanged(skipSilenceEnabled: Boolean) = listener.onSkipSilenceEnabledChanged(skipSilenceEnabled)
        override fun onDeviceInfoChanged(deviceInfo: DeviceInfo) = listener.onDeviceInfoChanged(deviceInfo)
        override fun onDeviceVolumeChanged(volume: Int, muted: Boolean) = listener.onDeviceVolumeChanged(volume, muted)
        override fun onVideoSizeChanged(videoSize: VideoSize) = listener.onVideoSizeChanged(videoSize)
        override fun onSurfaceSizeChanged(width: Int, height: Int) = listener.onSurfaceSizeChanged(width, height)
        override fun onRenderedFirstFrame() = listener.onRenderedFirstFrame()
        override fun onCues(cues: List<androidx.media3.common.text.Cue>) = listener.onCues(cues)
        override fun onCues(cueGroup: CueGroup) = listener.onCues(cueGroup)
        override fun onMetadata(metadata: Metadata) = listener.onMetadata(metadata)
    }

    private companion object {
        val Previous = intArrayOf(Player.COMMAND_SEEK_TO_PREVIOUS, Player.COMMAND_SEEK_TO_PREVIOUS_MEDIA_ITEM)
        val Next = intArrayOf(Player.COMMAND_SEEK_TO_NEXT, Player.COMMAND_SEEK_TO_NEXT_MEDIA_ITEM)
    }
}
