package com.mmcdole.kivo.api.models

import kotlinx.serialization.KSerializer
import kotlinx.serialization.Serializable
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder

// The contract's enums the apps branch on, written by hand: the generator
// maps each schema here (`make generate` in server/). Every other enum
// stays an open string. A value a later server adds decodes as Unknown, so
// an older app keeps working.

/** A track's type. */
@Serializable(with = TrackType.Serializer::class)
enum class TrackType(val value: String) {
    Video("video"), Audio("audio"), Subtitle("subtitle"), Unknown("");

    internal object Serializer : OpenEnumSerializer<TrackType>("TrackType", entries, Unknown, { it.value })
}

/** How a choice of a play shows: inside the `file`, in the `stream`, as an `external` file, `burn`ed in, or `none` for off. */
@Serializable(with = TrackDelivery.Serializer::class)
enum class TrackDelivery(val value: String) {
    File("file"), Stream("stream"), External("external"), Burn("burn"), None("none"), Unknown("");

    internal object Serializer : OpenEnumSerializer<TrackDelivery>("TrackDelivery", entries, Unknown, { it.value })
}

/** How a play's media is fetched: the original `file` or the HLS `stream`. */
@Serializable(with = PlayDelivery.Serializer::class)
enum class PlayDelivery(val value: String) {
    File("file"), Stream("stream"), Unknown("");

    internal object Serializer : OpenEnumSerializer<PlayDelivery>("PlayDelivery", entries, Unknown, { it.value })
}

/** What a download fetches: the original `file`, or a `conversion` made for it. */
@Serializable(with = DownloadDelivery.Serializer::class)
enum class DownloadDelivery(val value: String) {
    File("file"), Conversion("conversion"), Unknown("");

    internal object Serializer : OpenEnumSerializer<DownloadDelivery>("DownloadDelivery", entries, Unknown, { it.value })
}

/** When subtitles show: `none` never, `forced` only forced tracks, `smart` when the audio is in another language, or `always`. */
@Serializable(with = SubtitleMode.Serializer::class)
enum class SubtitleMode(val value: String) {
    None("none"), Forced("forced"), Smart("smart"), Always("always"), Unknown("");

    internal object Serializer : OpenEnumSerializer<SubtitleMode>("SubtitleMode", entries, Unknown, { it.value })
}

/** An enum as its wire string, decoding a string it doesn't know as [unknown]. */
internal open class OpenEnumSerializer<T : Enum<T>>(
    name: String,
    private val values: List<T>,
    private val unknown: T,
    private val wire: (T) -> String,
) : KSerializer<T> {
    override val descriptor: SerialDescriptor = PrimitiveSerialDescriptor("com.mmcdole.kivo.api.models.$name", PrimitiveKind.STRING)

    override fun deserialize(decoder: Decoder): T {
        val value = decoder.decodeString()
        return values.firstOrNull { it != unknown && wire(it) == value } ?: unknown
    }

    override fun serialize(encoder: Encoder, value: T) = encoder.encodeString(wire(value))
}
