package com.mmcdole.kivo.core.caps

import com.mmcdole.kivo.api.models.AudioEntry

// How each audio codec reaches the speakers: passed through to the TV or
// receiver, decoded on the device, or transcoded on the server, whichever
// comes first. The report's audio entries say which: a passed-through codec
// at the sink's channels, a decoded one at the device's channels, and a codec
// in neither is left to the server.

/** The Passthrough setting on Android TV. */
enum class PassthroughMode {
    /** Whatever the HDMI sink reports it takes. */
    Auto,

    /** Nothing: everything is decoded on the device. */
    Off,
}

/**
 * A format that can pass through, by the codec the server names it and the
 * most channels it carries. DTS-HD is the server's `dts` too: with it, a
 * DTS track passes through whole, at up to 7.1; with only DTS, its 5.1 core.
 */
enum class PassthroughFormat(val codec: String, val channels: Int) {
    Ac3("ac3", 6),
    Eac3("eac3", 8),
    Dts("dts", 6),
    DtsHd("dts", 8),
    TrueHd("truehd", 8),
}

/** This device's audio settings; phones, tablets and Apple devices have only [maxChannels]. */
data class AudioSettings(
    val passthrough: PassthroughMode = PassthroughMode.Auto,
    /** Maximum audio channels; null for Auto. */
    val maxChannels: Int? = null,
)

/** The audio output as the platform reports it. */
data class AudioSink(
    /** The formats the sink takes encoded, with the channels it takes of each. */
    val passthrough: Map<PassthroughFormat, Int> = emptyMap(),
    /** The PCM channels it outputs; 0 when it can't say. */
    val channels: Int = 0,
)

/**
 * The most channels the device outputs of what it decodes: the Maximum
 * audio channels setting, else the sink's PCM channels, else stereo.
 */
internal fun deviceChannels(setting: Int?, sink: AudioSink): Int = setting ?: sink.channels.takeIf { it > 0 } ?: 2

/** The codecs that pass through under [settings], each at the channels it reaches the sink with. */
internal fun passthroughCodecs(sink: AudioSink, settings: AudioSettings): Map<String, Int> {
    val formats = when (settings.passthrough) {
        PassthroughMode.Off -> emptyMap()
        PassthroughMode.Auto -> sink.passthrough
    }
    return formats.entries.groupBy({ it.key.codec }, { it.value }).mapValues { (_, channels) -> channels.max() }
}

/**
 * The report's audio entries, each in [delivery]: each passed-through codec
 * at its channels, and each codec decoded on the device capped at
 * [deviceChannels], never raised, so the server mixes down what the device
 * can't output.
 */
internal fun audioEntries(decoders: List<AudioAnswer>, sink: AudioSink, settings: AudioSettings, delivery: List<String>): List<AudioEntry> {
    val passed = passthroughCodecs(sink, settings)
    val channels = deviceChannels(settings.maxChannels, sink)
    val decoded = decoders.filter { it.codec !in passed }.map { AudioEntry(it.codec, delivery, minOf(it.channels, channels)) }
    return decoded + passed.map { (codec, max) -> AudioEntry(codec, delivery, max) }
}
